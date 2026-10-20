// Package acp implements a minimal Agent Client Protocol (ACP) server over
// stdio JSON-RPC (Spec 12c). It exposes initialize/session.new/session.prompt
// and streams agent output as session/update notifications.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Chunk is a piece of streamed agent output.
type Chunk struct {
	Text string
	Done bool
	Err  error
}

// Agent is the minimal agent surface ACP needs. The concrete opencode agent is
// adapted to it at the call site.
type Agent interface {
	Run(ctx context.Context, sessionID, content string) (<-chan Chunk, error)
	Cancel(sessionID string)
}

// Server speaks ACP over an in/out stream.
type Server struct {
	agent Agent

	mu     sync.Mutex
	out    *bufio.Writer
	in     io.Reader
	nextID int
	// sessions tracks sessions created in this process.
	sessions map[string]bool
}

// New builds an ACP server over in/out.
func New(agent Agent, in io.Reader, out io.Writer) *Server {
	return &Server{
		agent:    agent,
		out:      bufio.NewWriter(out),
		in:       in,
		sessions: map[string]bool{},
	}
}

// rpcRequest / rpcResponse are JSON-RPC 2.0 messages (newline-delimited).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// Run reads requests until EOF. It is safe to run once per server. Each request
// is dispatched in its own goroutine so a streaming prompt does not block reads.
func (s *Server) Run(ctx context.Context) error {
	sc := bufio.NewScanner(s.in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return s.serve(ctx, sc)
}

func (s *Server) serve(ctx context.Context, sc *bufio.Scanner) error {
	var wg sync.WaitGroup
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		wg.Add(1)
		go func(req rpcRequest) {
			defer wg.Done()
			s.dispatch(ctx, req)
		}(req)
	}
	wg.Wait()
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) {
	switch req.Method {
	case "initialize":
		s.reply(req.ID, map[string]any{
			"protocolVersion": 1,
			"agentCapabilities": map[string]any{
				"promptCapabilities": map[string]any{"image": false},
			},
		})
	case "session/new":
		s.handleSessionNew(req)
	case "session/prompt":
		s.handleSessionPrompt(ctx, req)
	case "session/cancel":
		s.handleSessionCancel(req)
	default:
		s.replyError(req.ID, -32601, "method not found")
	}
}

func (s *Server) handleSessionNew(req rpcRequest) {
	var params struct {
		Cwd string `json:"cwd"`
	}
	_ = json.Unmarshal(req.Params, &params)
	s.mu.Lock()
	s.nextID++
	id := fmt.Sprintf("sess-%d", s.nextID)
	s.sessions[id] = true
	s.mu.Unlock()
	s.reply(req.ID, map[string]any{"sessionId": id})
}

func (s *Server) handleSessionPrompt(ctx context.Context, req rpcRequest) {
	var params struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.replyError(req.ID, -32602, "invalid params")
		return
	}
	s.mu.Lock()
	known := s.sessions[params.SessionID]
	s.mu.Unlock()
	if !known {
		s.replyError(req.ID, -32602, "unknown session")
		return
	}
	text := ""
	for _, p := range params.Prompt {
		text += p.Text
	}
	ch, err := s.agent.Run(ctx, params.SessionID, text)
	if err != nil {
		s.replyError(req.ID, -32603, err.Error())
		return
	}
	// Stream chunks as session/update notifications, then reply.
	for chunk := range ch {
		if chunk.Err != nil {
			s.replyError(req.ID, -32603, chunk.Err.Error())
			return
		}
		if chunk.Text != "" {
			s.notify("session/update", map[string]any{
				"sessionId": params.SessionID,
				"content":   map[string]any{"type": "text", "text": chunk.Text},
			})
		}
	}
	s.reply(req.ID, map[string]any{"stopReason": "end_turn"})
}

func (s *Server) handleSessionCancel(req rpcRequest) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(req.Params, &params)
	s.agent.Cancel(params.SessionID)
	s.reply(req.ID, map[string]any{"cancelled": true})
}

func (s *Server) reply(id json.RawMessage, result any) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) replyError(id json.RawMessage, code int, msg string) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func (s *Server) notify(method string, params any) {
	s.write(notification{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.out.Write(data)
	s.out.WriteByte('\n')
	s.out.Flush()
}
