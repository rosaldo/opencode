package config

import (
	"path/filepath"
	"testing"
)

func TestPluginsParse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"plugins": [
			"opencode-example-plugin",
			{ "package": "./plugins/local", "options": { "enabled": true } },
			"-disabled-one"
		]
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if len(loaded.Plugins) != 3 {
		t.Fatalf("plugins = %+v", loaded.Plugins)
	}
	if loaded.Plugins[0].Package != "opencode-example-plugin" || loaded.Plugins[0].Disabled {
		t.Fatalf("plugins[0] = %+v", loaded.Plugins[0])
	}
	if loaded.Plugins[1].Package != "./plugins/local" || loaded.Plugins[1].Disabled {
		t.Fatalf("plugins[1] = %+v", loaded.Plugins[1])
	}
	if loaded.Plugins[1].Options["enabled"] != true {
		t.Fatalf("plugins[1].options = %+v", loaded.Plugins[1].Options)
	}
	if loaded.Plugins[2].Package != "disabled-one" || !loaded.Plugins[2].Disabled {
		t.Fatalf("plugins[2] = %+v", loaded.Plugins[2])
	}
}
