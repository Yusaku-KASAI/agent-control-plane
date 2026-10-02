package docker

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeclaredPluginDirs(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), `{
  "extraKnownMarketplaces": {
    "local": {"source": {"source": "directory", "path": "./platform"}},
    "remote": {"source": {"source": "github", "repo": "someone/plugins"}}
  },
  "enabledPlugins": {
    "roles@local": true,
    "slack@local": true,
    "off@local": false,
    "other@remote": true,
    "builtin@claude-plugins-official": true
  }
}`)
	writeFile(t, filepath.Join(repo, "platform", ".claude-plugin", "marketplace.json"), `{
  "name": "local",
  "plugins": [
    {"name": "roles", "source": "./plugins/roles"},
    {"name": "slack", "source": "./plugins/slack"},
    {"name": "off", "source": "./plugins/off"}
  ]
}`)

	got, err := declaredPluginDirs(repo)
	if err != nil {
		t.Fatalf("declaredPluginDirs() error = %v", err)
	}
	want := []string{"platform/plugins/roles", "platform/plugins/slack"}
	if !slices.Equal(got, want) {
		t.Errorf("declaredPluginDirs() = %q, want %q", got, want)
	}
}

func TestDeclaredPluginDirs_Errors(t *testing.T) {
	tests := map[string]struct {
		settings, catalog string
	}{
		"plugin missing from the marketplace": {
			settings: `{"extraKnownMarketplaces": {"m": {"source": {"source": "directory", "path": "./p"}}}, "enabledPlugins": {"x@m": true}}`,
			catalog:  `{"plugins": []}`,
		},
		"plugin source escapes the marketplace": {
			settings: `{"extraKnownMarketplaces": {"m": {"source": {"source": "directory", "path": "./p"}}}, "enabledPlugins": {"x@m": true}}`,
			catalog:  `{"plugins": [{"name": "x", "source": "../../etc"}]}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			writeFile(t, filepath.Join(repo, ".claude", "settings.json"), tt.settings)
			writeFile(t, filepath.Join(repo, "p", ".claude-plugin", "marketplace.json"), tt.catalog)
			if _, err := declaredPluginDirs(repo); err == nil {
				t.Fatal("declaredPluginDirs() error = nil, want error")
			}
		})
	}
}

func TestDeclaredPluginDirs_NoSettings(t *testing.T) {
	got, err := declaredPluginDirs(t.TempDir())
	if err != nil || got != nil {
		t.Errorf("declaredPluginDirs() = %q, %v; want nothing", got, err)
	}
}

func TestDeclaredPluginDirs_ThisRepository(t *testing.T) {
	got, err := declaredPluginDirs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("declaredPluginDirs() error = %v", err)
	}
	if !slices.Contains(got, "platform/plugins/roles") {
		t.Errorf("declaredPluginDirs() = %q, want the roles plugin", got)
	}
}
