package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/llm/agent"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/pubsub"
	"github.com/opencode-ai/opencode/internal/session"

	"github.com/google/uuid"
)

// AgentManager owns the set of user-selectable agents. It lazily instantiates
// agent.Service values (one provider per agent) and exposes the active one
// through the same agent.Service surface the app used before it existed, so
// call sites that only Run/IsBusy/Model keep working unchanged.
//
// Session continuity: switching agents does not reset the session; the next
// turn is answered by the new agent with the same message history (parity with
// the TypeScript version). Cancellation, busy checks and the event stream
// consider every instantiated agent, not just the active one.
type AgentManager struct {
	mu     sync.RWMutex
	active string
	cache  map[string]agent.Service

	deps    agent.AgentToolDeps
	ctx     context.Context
	cancel  context.CancelFunc
	broker  *pubsub.Broker[agent.AgentEvent]
	fanInWG sync.WaitGroup
}

// NewAgentManager creates a manager whose active agent is the registry
// default (or "coder"). The default agent is instantiated eagerly so
// misconfiguration surfaces at boot; every other agent is created on demand.
func NewAgentManager(ctx context.Context, deps agent.AgentToolDeps) *AgentManager {
	ctx, cancel := context.WithCancel(ctx)
	m := &AgentManager{
		active: agentdef.Get().Default(),
		cache:  make(map[string]agent.Service),
		deps:   deps,
		ctx:    ctx,
		cancel: cancel,
		broker: pubsub.NewBroker[agent.AgentEvent](),
	}
	if m.active == "" {
		m.active = agentdef.DefaultName
	}
	if _, err := m.ActiveAgent(); err != nil {
		// Keep boot resilient: log and leave the cache empty; the next
		// Run/ActiveAgent call retries (e.g. after the provider is fixed).
		logging.Error("Failed to instantiate default agent", err, "agent", m.active)
	}
	return m
}

// Active returns the name of the active primary agent.
func (m *AgentManager) Active() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ActiveAgent returns the agent.Service for the active agent, creating and
// caching it on first use.
func (m *AgentManager) ActiveAgent() (agent.Service, error) {
	m.mu.RLock()
	name := m.active
	m.mu.RUnlock()
	return m.get(name)
}

// Switch changes the active agent after validating it is selectable:
// registered, enabled, not hidden and not a subagent. Instantiation stays
// lazy — the provider is created when the agent is first used.
func (m *AgentManager) Switch(name string) error {
	def, ok := agentdef.Get().Get(name)
	if !ok {
		return fmt.Errorf("unknown agent %q", name)
	}
	if def.Disabled {
		return fmt.Errorf("agent %q is disabled", name)
	}
	if def.Hidden {
		return fmt.Errorf("agent %q is hidden", name)
	}
	if def.Mode == agentdef.ModeSubagent {
		return fmt.Errorf("agent %q is a subagent and cannot be selected", name)
	}
	m.mu.Lock()
	m.active = name
	m.mu.Unlock()
	return nil
}

// get returns the cached service for name, instantiating it under a
// single-flight lock on first use.
func (m *AgentManager) get(name string) (agent.Service, error) {
	m.mu.RLock()
	svc, ok := m.cache[name]
	m.mu.RUnlock()
	if ok {
		return svc, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if svc, ok := m.cache[name]; ok {
		return svc, nil
	}
	def, _ := agentdef.Get().Get(name)
	svc, err := agent.NewAgent(
		name,
		m.deps.Sessions,
		m.deps.Messages,
		m.deps.Snapshots,
		m.deps.SnapshotFor,
		agent.ToolsForDefinition(def, m.deps),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent %q: %w", name, err)
	}
	m.cache[name] = svc
	m.fanIn(name, svc)
	return svc, nil
}

// fanIn republishes every event of a lazily created agent on the manager
// broker so TUI subscribers keep receiving events regardless of which agent
// produced them.
func (m *AgentManager) fanIn(name string, svc agent.Service) {
	m.fanInWG.Add(1)
	go func() {
		defer logging.RecoverPanic("agent-manager-fanIn", nil)
		defer m.fanInWG.Done()
		events := svc.Subscribe(m.ctx)
		for ev := range events {
			m.broker.Publish(ev.Type, ev.Payload)
		}
		logging.Debug("agent event fan-in finished", "agent", name)
	}()
}

// Shutdown stops the event fan-in. It cancels the manager context first: the
// fan-in goroutines only exit when their subscription channel closes, which
// happens on context cancellation (or on broker shutdown).
func (m *AgentManager) Shutdown() {
	if m.cancel != nil {
		m.cancel()
	}
	m.fanInWG.Wait()
	m.broker.Shutdown()
}

// --- agent.Service implementation, delegating to the active agent ---

// Subscribe returns the manager event stream (fan-in of every agent).
func (m *AgentManager) Subscribe(ctx context.Context) <-chan pubsub.Event[agent.AgentEvent] {
	return m.broker.Subscribe(ctx)
}

// Model returns the model of the active agent.
func (m *AgentManager) Model() models.Model {
	svc, err := m.ActiveAgent()
	if err != nil {
		return models.Model{}
	}
	return svc.Model()
}

// Run sends a message with the active agent.
func (m *AgentManager) Run(ctx context.Context, sessionID string, content string, attachments ...message.Attachment) (<-chan agent.AgentEvent, error) {
	svc, err := m.ActiveAgent()
	if err != nil {
		return nil, err
	}
	return svc.Run(ctx, sessionID, content, attachments...)
}

// SubagentLookup reports whether name is a registered, enabled agent that can
// be invoked as a subagent, returning its definition.
func (m *AgentManager) SubagentLookup(name string) (*agentdef.Definition, bool) {
	def, ok := agentdef.Get().Get(name)
	if !ok || def.Disabled {
		return nil, false
	}
	return def, true
}

// RunSubagent launches a named agent on prompt inside a new child session of
// parentSessionID, mirroring the tool-based delegation path but driven by an
// @-mention. The child session is stored with the parent link so it can be
// navigated from the session list. The returned channel receives the agent's
// events; the child session is created before running.
func (m *AgentManager) RunSubagent(ctx context.Context, parentSessionID, name, prompt string) (session.Session, <-chan agent.AgentEvent, error) {
	def, ok := m.SubagentLookup(name)
	if !ok {
		return session.Session{}, nil, fmt.Errorf("unknown subagent %q", name)
	}
	svc, err := m.get(def.Name)
	if err != nil {
		return session.Session{}, nil, err
	}
	sess, err := m.deps.Sessions.CreateTaskSession(ctx, uuid.New().String(), parentSessionID, "New Agent Session")
	if err != nil {
		return session.Session{}, nil, fmt.Errorf("error creating session: %w", err)
	}
	events, err := svc.Run(ctx, sess.ID, prompt)
	if err != nil {
		return session.Session{}, nil, err
	}
	return sess, events, nil
}

// Cancel cancels any in-flight request for sessionID across all instantiated
// agents (the session may have been started by a different agent before a
// switch).
func (m *AgentManager) Cancel(sessionID string) {
	for _, svc := range m.snapshot() {
		svc.Cancel(sessionID)
	}
}

// IsSessionBusy reports whether any instantiated agent is processing sessionID.
func (m *AgentManager) IsSessionBusy(sessionID string) bool {
	for _, svc := range m.snapshot() {
		if svc.IsSessionBusy(sessionID) {
			return true
		}
	}
	return false
}

// IsBusy reports whether any instantiated agent is busy, so switching agents
// mid-turn never allows a second concurrent generation.
func (m *AgentManager) IsBusy() bool {
	for _, svc := range m.snapshot() {
		if svc.IsBusy() {
			return true
		}
	}
	return false
}

// Update applies a model change to the named agent (or the active one when
// name is empty), refreshing its provider.
func (m *AgentManager) Update(name string, modelID models.ModelID) (models.Model, error) {
	if name == "" {
		name = m.Active()
	}
	svc, err := m.get(name)
	if err != nil {
		return models.Model{}, err
	}
	return svc.Update(name, modelID)
}

// Summarize compacts the session with the active agent's summarize provider.
func (m *AgentManager) Summarize(ctx context.Context, sessionID string) error {
	svc, err := m.ActiveAgent()
	if err != nil {
		return err
	}
	return svc.Summarize(ctx, sessionID)
}

func (m *AgentManager) snapshot() []agent.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]agent.Service, 0, len(m.cache))
	for _, svc := range m.cache {
		out = append(out, svc)
	}
	return out
}
