package app

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/llm/agent"
)

func withRegistry(t *testing.T, defs ...*agentdef.Definition) {
	t.Helper()
	prev := agentdef.Get()
	reg := agentdef.NewRegistry()
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	agentdef.Set(reg)
	t.Cleanup(func() { agentdef.Set(prev) })
}

func newTestManager() *AgentManager {
	return &AgentManager{cache: make(map[string]agent.Service)}
}

func TestSwitchAcceptsPrimaryAndAll(t *testing.T) {
	withRegistry(t,
		&agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: "writer", Mode: agentdef.ModeAll},
	)
	m := newTestManager()

	for _, name := range []string{"coder", "writer"} {
		if err := m.Switch(name); err != nil {
			t.Fatalf("Switch(%q) = %v, want nil", name, err)
		}
		if got := m.Active(); got != name {
			t.Fatalf("Active() = %q, want %q", got, name)
		}
	}
}

func TestSwitchRejectsSubagent(t *testing.T) {
	withRegistry(t,
		&agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent},
	)
	m := newTestManager()

	if err := m.Switch("reviewer"); err == nil {
		t.Fatal("Switch(subagent) = nil, want error")
	}
	if got := m.Active(); got != "" {
		t.Fatalf("Active() = %q after failed switch, want unchanged", got)
	}
}

func TestSwitchRejectsHiddenAndDisabled(t *testing.T) {
	withRegistry(t,
		&agentdef.Definition{Name: "ghost", Mode: agentdef.ModePrimary, Hidden: true},
		&agentdef.Definition{Name: "retired", Mode: agentdef.ModePrimary, Disabled: true},
	)
	m := newTestManager()

	if err := m.Switch("ghost"); err == nil {
		t.Fatal("Switch(hidden) = nil, want error")
	}
	if err := m.Switch("retired"); err == nil {
		t.Fatal("Switch(disabled) = nil, want error")
	}
}

func TestSwitchRejectsUnknown(t *testing.T) {
	withRegistry(t, &agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary})
	m := newTestManager()

	if err := m.Switch("nope"); err == nil {
		t.Fatal("Switch(unknown) = nil, want error")
	}
}

func TestSwitchDoesNotInstantiate(t *testing.T) {
	withRegistry(t,
		&agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: "writer", Mode: agentdef.ModePrimary},
	)
	m := newTestManager()

	if err := m.Switch("writer"); err != nil {
		t.Fatalf("Switch(writer) = %v", err)
	}
	if len(m.cache) != 0 {
		t.Fatalf("cache has %d entries after Switch, want 0 (lazy instantiation)", len(m.cache))
	}
}

func TestGetReturnsCachedInstance(t *testing.T) {
	withRegistry(t, &agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary})
	m := newTestManager()

	stub := &stubAgent{}
	m.cache["coder"] = stub

	first, err := m.get("coder")
	if err != nil {
		t.Fatalf("get(coder) = %v", err)
	}
	second, err := m.get("coder")
	if err != nil {
		t.Fatalf("get(coder) again = %v", err)
	}
	if first != second || first != agent.Service(stub) {
		t.Fatal("get did not return the cached instance")
	}
}

func TestDefaultActiveComesFromRegistry(t *testing.T) {
	withRegistry(t,
		&agentdef.Definition{Name: "writer", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary},
	)
	agentdef.Get().SetDefault("writer")

	// NewAgentManager eagerly instantiates the default agent, which needs a
	// loaded config; only the active-name resolution is under test here, so
	// replicate its selection logic without the constructor.
	m := newTestManager()
	m.active = agentdef.Get().Default()
	if m.Active() != "writer" {
		t.Fatalf("Active() = %q, want %q", m.Active(), "writer")
	}
}

// stubAgent implements agent.Service for cache identity checks; the unused
// methods are never called by the manager paths under test.
type stubAgent struct{ agent.Service }
