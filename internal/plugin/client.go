package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// HookResult is the verdict returned by a hook invocation. A nil Allow means the
// hook abstained (does not decide). Deny short-circuits to a denial.
type HookResult struct {
	Allow     *bool          `json:"allow,omitempty"`
	Message   string         `json:"message,omitempty"`
	Mutate    map[string]any `json:"mutate,omitempty"`
	Model     string         `json:"model,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

// rpcRequest / rpcResponse are the line-delimited JSON messages of the protocol.
type rpcRequest struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Client is a running plugin process speaking the stdio JSON protocol.
type Client struct {
	plugin Plugin

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.ReadCloser
	nextID int
	closed bool
}

// Start spawns the plugin process. A plugin without a command is declarative and
// yields a client with no process (Start returns a usable no-op client).
func Start(p Plugin) (*Client, error) {
	c := &Client{plugin: p}
	if len(p.Manifest.Command) == 0 {
		return c, nil
	}
	cmd := exec.Command(p.Manifest.Command[0], p.Manifest.Command[1:]...)
	cmd.Dir = p.Dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = sc
	c.stderr = stderr
	return c, nil
}

// Name returns the plugin name.
func (c *Client) Name() string { return c.plugin.Name() }

// Call invokes a method with params and returns the raw result. It is safe for
// concurrent use.
func (c *Client) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if c.cmd == nil {
		return nil, nil // declarative plugin: nothing to call
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("plugin client closed")
	}
	c.nextID++
	id := c.nextID
	req := rpcRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}

	type scanResult struct {
		resp rpcResponse
		err  error
	}
	ch := make(chan scanResult, 1)
	go func() {
		for c.stdout.Scan() {
			line := c.stdout.Bytes()
			if len(line) == 0 {
				continue
			}
			var resp rpcResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			if resp.ID == id {
				ch <- scanResult{resp: resp}
				return
			}
		}
		ch <- scanResult{err: io.EOF}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		if r.resp.Error != "" {
			return nil, fmt.Errorf("plugin %s: %s", c.Name(), r.resp.Error)
		}
		return r.resp.Result, nil
	}
}

// RunHook invokes a hook with the given payload and decodes its result.
func (c *Client) RunHook(ctx context.Context, hook Hook, payload map[string]any) (HookResult, error) {
	raw, err := c.Call(ctx, "hook", map[string]any{"hook": string(hook), "payload": payload})
	if err != nil {
		return HookResult{}, err
	}
	if len(raw) == 0 {
		return HookResult{}, nil
	}
	var res HookResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return HookResult{}, err
	}
	return res, nil
}

// Close terminates the plugin process.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
