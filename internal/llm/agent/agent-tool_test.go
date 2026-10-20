package agent

import (
	"strings"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
)

func TestCanDelegateTo(t *testing.T) {
	t.Parallel()

	coder := &agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary}
	task := &agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true}
	title := &agentdef.Definition{Name: string(config.AgentTitle), Mode: agentdef.ModeSubagent, Hidden: true}
	primary := &agentdef.Definition{Name: "planner", Mode: agentdef.ModePrimary}
	disabled := &agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeAll, Disabled: true}

	if !canDelegateTo(coder, task, nil) {
		t.Error("the hidden task builtin must stay invokable")
	}
	if !canDelegateTo(coder, primary, nil) {
		t.Error("any registered agent must be invokable, whatever its role")
	}
	if canDelegateTo(coder, coder, nil) {
		t.Error("an agent must not delegate to itself")
	}
	if canDelegateTo(coder, disabled, nil) {
		t.Error("disabled agents must not be invokable")
	}
	if canDelegateTo(coder, title, nil) {
		t.Error("internal helpers must not be invokable")
	}
	if canDelegateTo(coder, task, []string{string(config.AgentTask)}) {
		t.Error("an agent on the delegation chain must not be re-entered")
	}
	if !canDelegateTo(coder, task, []string{"planner"}) {
		t.Error("the chain must only block the agents it contains")
	}
	if !canDelegateTo(nil, task, nil) {
		t.Error("a callerless tool must still resolve targets")
	}
}

func TestSubagentsListing(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent, Description: "Reviews\n  code"},
		&agentdef.Definition{Name: "auditor", Mode: agentdef.ModeAll, Hidden: true},
		&agentdef.Definition{Name: "planner", Mode: agentdef.ModeAll},
		&agentdef.Definition{Name: "lead", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: "retired", Mode: agentdef.ModeAll, Disabled: true},
	)
	caller, ok := agentdef.Get().Get(string(config.AgentCoder))
	if !ok {
		t.Fatal("coder missing from the test registry")
	}

	visible := subagents(caller, false, nil)
	names := make(map[string]bool, len(visible))
	for _, def := range visible {
		names[def.Name] = true
	}
	// Primary agents are invokable too: an orchestrator may run any other agent.
	if len(visible) != 3 || !names["reviewer"] || !names["planner"] || !names["lead"] {
		t.Fatalf("visible subagents = %v, want reviewer, planner and lead", names)
	}

	if len(subagents(caller, true, nil)) != 5 {
		t.Fatalf("invokable subagents = %d, want 5 (reviewer, auditor, planner, lead, task)", len(subagents(caller, true, nil)))
	}

	// The chain removes its members from both the listing and the eligibility.
	chained := subagents(caller, true, []string{"planner", string(config.AgentTask)})
	if len(chained) != 3 {
		t.Fatalf("invokable subagents with a chain = %d, want 3 (reviewer, auditor, lead)", len(chained))
	}
}

func TestSubagentsListingHonoursTaskPolicy(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent},
		&agentdef.Definition{Name: "planner", Mode: agentdef.ModeAll},
	)
	caller := &agentdef.Definition{
		Name:        "restricted",
		Mode:        agentdef.ModeAll,
		Permissions: []agentdef.PermissionRule{{Action: "task", Resource: "reviewer", Effect: "allow"}, {Action: "task", Resource: "", Effect: "deny"}},
	}

	listed := subagents(caller, false, nil)
	if len(listed) != 1 || listed[0].Name != "reviewer" {
		names := make([]string, 0, len(listed))
		for _, def := range listed {
			names = append(names, def.Name)
		}
		t.Fatalf("listed subagents = %v, want only reviewer", names)
	}

	// The denied planner is not invokable either; the hidden task is only
	// reachable through the "*" rule, which denies it.
	if len(subagents(caller, true, nil)) != 1 {
		t.Fatalf("invokable subagents = %d, want only reviewer", len(subagents(caller, true, nil)))
	}
}

func TestToolDescription(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary, Team: "core"},
		&agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true, Description: "Read-only search"},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent, Team: "core", Description: "Reviews  code\nand tests"},
		&agentdef.Definition{Name: "deployer", Mode: agentdef.ModeAll, Team: "ops", Description: "Deploys stuff"},
	)
	caller, _ := agentdef.Get().Get(string(config.AgentCoder))

	tool := NewAgentTool(caller, AgentToolDeps{})
	info := tool.Info()

	if !strings.Contains(info.Description, "Your department") {
		t.Fatalf("description must list the caller's department:\n%s", info.Description)
	}
	if !strings.Contains(info.Description, "reviewer — Reviews code and tests") {
		t.Fatalf("description must flatten the subagent description:\n%s", info.Description)
	}
	if strings.Contains(info.Description, "task —") {
		t.Fatalf("hidden agents must not be advertised:\n%s", info.Description)
	}
	if !strings.Contains(info.Description, "Other departments (names only):") {
		t.Fatalf("description must list other departments by name:\n%s", info.Description)
	}
	if !strings.Contains(info.Description, "- ops: deployer") {
		t.Fatalf("other departments must be grouped by team:\n%s", info.Description)
	}
	if !strings.Contains(info.Description, "Any registered agent may be run by name") {
		t.Fatalf("description must say that unlisted agents are invokable too:\n%s", info.Description)
	}
	if _, ok := info.Parameters["subagent"]; !ok {
		t.Error("the tool must declare the subagent parameter")
	}
	if len(info.Required) != 1 || info.Required[0] != "prompt" {
		t.Errorf("required = %v, want only prompt", info.Required)
	}
}

func TestToolDescriptionAllDepartments(t *testing.T) {
	// A caller without a department lists everyone by name; a large registry
	// still costs only names in the description.
	defs := []*agentdef.Definition{
		{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary},
	}
	for i := 0; i < 60; i++ {
		defs = append(defs, &agentdef.Definition{
			Name: "agent-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Mode: agentdef.ModeAll,
			Team: "dept-" + string(rune('a'+i%5)),
		})
	}
	withTestRegistry(t, defs...)
	caller, _ := agentdef.Get().Get(string(config.AgentCoder))

	description := NewAgentTool(caller, AgentToolDeps{}).Info().Description
	// No registry size may hide an agent: every name appears in the listing.
	for i := 0; i < 60; i++ {
		name := "agent-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if !strings.Contains(description, name) {
			t.Errorf("subagent %s missing from the description:\n%s", name, description)
		}
	}
	// Other departments are listed by name only.
	if strings.Contains(description, "— ") {
		t.Errorf("departments other than the caller's must be listed by name only:\n%s", description)
	}
	if strings.Contains(description, "Your department") {
		t.Errorf("a caller without a department must not have an own section:\n%s", description)
	}
}

func TestToolDescriptionOmitsChainAgents(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: "orchestrator", Mode: agentdef.ModeAll, Team: "core"},
		&agentdef.Definition{Name: "planner", Mode: agentdef.ModePrimary, Team: "core"},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent, Team: "core"},
	)
	caller, _ := agentdef.Get().Get("orchestrator")

	description := NewAgentTool(caller, AgentToolDeps{DelegationChain: []string{"planner"}}).Info().Description
	if strings.Contains(description, "- planner —") {
		t.Fatalf("the chained planner must not be advertised:\n%s", description)
	}
	if !strings.Contains(description, "- reviewer —") {
		t.Fatalf("the reviewer must stay advertised:\n%s", description)
	}
}

func TestAgentToolAvailability(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true},
	)
	coder, _ := agentdef.Get().Get(string(config.AgentCoder))

	if agentToolFor(coder, AgentToolDeps{}) == nil {
		t.Error("coder must get the delegation tool while task exists")
	}
	if agentToolFor(nil, AgentToolDeps{}) != nil {
		t.Error("an unknown caller must not get the delegation tool")
	}
	if agentToolFor(&agentdef.Definition{Name: "x", Mode: agentdef.ModeSubagent}, AgentToolDeps{}) != nil {
		t.Error("subagent mode agents must not delegate further")
	}
	if agentToolFor(&agentdef.Definition{Name: string(config.AgentTitle), Mode: agentdef.ModeSubagent}, AgentToolDeps{}) != nil {
		t.Error("title must not get the delegation tool")
	}

	denied := &agentdef.Definition{
		Name:        "isolated",
		Mode:        agentdef.ModeAll,
		Permissions: []agentdef.PermissionRule{{Action: "task", Resource: "", Effect: "deny"}},
	}
	if agentToolFor(denied, AgentToolDeps{}) != nil {
		t.Error("an agent that denies every subagent must not get the delegation tool")
	}

	// The only remaining invokable agent is on the chain: nothing left to run.
	if agentToolFor(coder, AgentToolDeps{DelegationChain: []string{string(config.AgentTask)}}) != nil {
		t.Error("with every subagent on the chain the delegation tool must be unavailable")
	}

	withTestRegistry(t, &agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary})
	if agentToolFor(coder, AgentToolDeps{}) != nil {
		t.Error("without any subagent the delegation tool must be unavailable")
	}
}

func TestSubagentByName(t *testing.T) {
	withTestRegistry(t,
		&agentdef.Definition{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true},
		&agentdef.Definition{Name: "reviewer", Mode: agentdef.ModeSubagent},
		&agentdef.Definition{Name: "planner", Mode: agentdef.ModePrimary},
		&agentdef.Definition{Name: string(config.AgentTitle), Mode: agentdef.ModeSubagent, Hidden: true},
	)
	coder, _ := agentdef.Get().Get(string(config.AgentCoder))
	tool := &agentTool{caller: coder}

	def, ok := tool.subagentByName(DefaultSubagent)
	if !ok || def.Name != string(config.AgentTask) {
		t.Fatalf("the default subagent must resolve to task, got %v %v", def, ok)
	}
	if _, ok := tool.subagentByName("reviewer"); !ok {
		t.Error("reviewer must be invokable")
	}
	if _, ok := tool.subagentByName("planner"); !ok {
		t.Error("primary agents must be invokable by an orchestrator")
	}
	if _, ok := tool.subagentByName(string(config.AgentTitle)); ok {
		t.Error("title must not be invokable")
	}
	if _, ok := tool.subagentByName(string(config.AgentCoder)); ok {
		t.Error("an agent must not be invokable by itself")
	}
	if _, ok := tool.subagentByName("does-not-exist"); ok {
		t.Error("unknown names must not resolve")
	}

	chained := &agentTool{caller: coder, chain: []string{"planner"}}
	if _, ok := chained.subagentByName("planner"); ok {
		t.Error("an agent on the delegation chain must not be invokable")
	}
	if _, ok := chained.subagentByName("reviewer"); !ok {
		t.Error("the chain must not block other agents")
	}
}

func TestChildDepsExtendsChain(t *testing.T) {
	tool := &agentTool{
		caller: &agentdef.Definition{Name: "planner", Mode: agentdef.ModePrimary},
		deps:   AgentToolDeps{DelegationChain: []string{"orchestrator"}},
	}
	child := tool.childDeps()
	if len(child.DelegationChain) != 2 ||
		child.DelegationChain[0] != "orchestrator" ||
		child.DelegationChain[1] != "planner" {
		t.Fatalf("child chain = %v, want [orchestrator planner]", child.DelegationChain)
	}
	if len(tool.deps.DelegationChain) != 1 {
		t.Fatalf("the parent chain must not be mutated, got %v", tool.deps.DelegationChain)
	}
}
