package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/opencode-ai/opencode/internal/config"
)

// TestHelperProcess is not a real test: it acts as a fake plugin executable when
// GO_WANT_HELPER_PROCESS=1. It echoes an id-matched response for each request.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	out := os.Stdout
	for sc.Scan() {
		line := sc.Bytes()
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		var result any
		switch req.Method {
		case "hook":
			hook, _ := req.Params["hook"].(string)
			switch hook {
			case string(HookToolBeforeExecute):
				tool, _ := req.Params["payload"].(map[string]any)["tool"].(string)
				if tool == "bash" {
					result = map[string]any{"allow": false, "message": "no bash for you"}
				} else {
					result = map[string]any{"allow": true, "mutate": map[string]any{"mutated": true}}
				}
			case string(HookChatParams):
				result = map[string]any{"model": "plugin/model"}
			default:
				result = map[string]any{}
			}
		default:
			result = map[string]any{}
		}
		resp := map[string]any{"id": req.ID, "result": result}
		b, _ := json.Marshal(resp)
		fmt.Fprintln(out, string(b))
	}
	os.Exit(0)
}

func helperClient(t *testing.T, hooks ...Hook) *Client {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	p := Plugin{
		Manifest: Manifest{
			Name:    "helper",
			Command: []string{os.Args[0], "-test.run=TestHelperProcess", "--"},
			Hooks:   hooks,
		},
	}
	c, err := Start(p)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestClientRunHookDeny(t *testing.T) {
	c := helperClient(t, HookToolBeforeExecute)
	res, err := c.RunHook(context.Background(), HookToolBeforeExecute, map[string]any{"tool": "bash"})
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if res.Allow == nil || *res.Allow {
		t.Fatalf("expected deny, got %+v", res)
	}
	if res.Message != "no bash for you" {
		t.Fatalf("message = %q", res.Message)
	}
}

func TestManagerHookIntegration(t *testing.T) {
	c := helperClient(t, HookToolBeforeExecute, HookChatParams)
	m := &Manager{clients: []*Client{c}}

	deny := m.RunToolBeforeExecute(context.Background(), "bash", nil)
	if deny.Allow {
		t.Fatal("bash should be denied")
	}
	allow := m.RunToolBeforeExecute(context.Background(), "read", nil)
	if !allow.Allow {
		t.Fatal("read should be allowed")
	}
	if allow.Mutate["mutated"] != true {
		t.Fatalf("mutate = %v", allow.Mutate)
	}

	cp := m.RunChatParams(context.Background(), "orig/model", nil)
	if cp.Model != "plugin/model" {
		t.Fatalf("chat model = %q", cp.Model)
	}
}

func TestManagerLoadIsolatesFailures(t *testing.T) {
	dir := t.TempDir()
	// A manifest pointing at a non-existent binary must not break loading.
	if err := os.MkdirAll(dir+"/bad", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir+"/bad/plugin.json", []byte(`{"name":"bad","command":["/nonexistent/binary"]}`), 0o644)

	m := &Manager{loader: &Loader{ProjectDir: dir}}
	warns := m.Load(nil)
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %v", warns)
	}
	if len(m.clients) != 0 {
		t.Fatalf("clients = %v", m.clients)
	}

	// A declarative plugin (no command) loads fine.
	if err := os.MkdirAll(dir+"/ok", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir+"/ok/plugin.json", []byte(`{"name":"ok"}`), 0o644)
	m2 := &Manager{loader: &Loader{ProjectDir: dir}}
	m2.Load([]config.PluginSpec{{Package: "ok"}})
	if got := clientNames(m2.clients); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("clients = %v, want [ok]", got)
	}
}

func clientNames(cs []*Client) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name())
	}
	return out
}
