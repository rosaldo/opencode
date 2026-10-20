package config

import (
	"path/filepath"
	"testing"
)

func TestPermissionsAndPoliciesParse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"permissions": [
			{"action": "shell", "resource": "*", "effect": "ask"},
			{"action": "shell", "resource": "git push *", "effect": "deny"},
			{"action": "read", "resource": "*.env", "effect": "deny"}
		],
		"policies": [
			{"action": "provider.use", "resource": "*", "effect": "deny"},
			{"action": "provider.use", "resource": "anthropic", "effect": "allow"},
			{"action": "tool.use", "resource": "shell:git push *", "effect": "deny"}
		]
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if len(loaded.Permissions) != 3 {
		t.Fatalf("permissions = %d, want 3", len(loaded.Permissions))
	}
	if got := loaded.Permissions[1]; got.Action != "shell" || got.Resource != "git push *" || got.Effect != "deny" {
		t.Fatalf("permissions[1] = %+v", got)
	}
	if len(loaded.Policies) != 3 {
		t.Fatalf("policies = %d, want 3", len(loaded.Policies))
	}
	if got := loaded.Policies[0]; got.Action != "provider.use" || got.Effect != "deny" {
		t.Fatalf("policies[0] = %+v", got)
	}
}
