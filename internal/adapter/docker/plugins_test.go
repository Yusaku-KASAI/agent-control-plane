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

func TestDeclaredPlugins(t *testing.T) {
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

	got, err := declaredPlugins(repo, filepath.Dir(repo), "/workspace")
	if err != nil {
		t.Fatalf("declaredPlugins() error = %v", err)
	}
	want := []pluginDir{{Path: "platform/plugins/roles"}, {Path: "platform/plugins/slack"}}
	if !slices.Equal(got, want) {
		t.Errorf("declaredPlugins() = %+v, want %+v", got, want)
	}
}

func TestDeclaredPlugins_BesideRepository(t *testing.T) {
	hostRoot := t.TempDir()
	// The session's clone may live elsewhere; only its name has to match the target's.
	repo := filepath.Join(t.TempDir(), "app")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), `{
  "extraKnownMarketplaces": {
    "shared": {"source": {"source": "directory", "path": "/workspace/kit/platform"}},
    "self": {"source": {"source": "directory", "path": "/workspace/app/own"}},
    "escape": {"source": {"source": "directory", "path": "/workspace/../etc"}},
    "elsewhere": {"source": {"source": "directory", "path": "/opt/plugins"}}
  },
  "enabledPlugins": {"roles@shared": true, "a@self": true, "b@escape": true, "c@elsewhere": true}
}`)
	writeFile(t, filepath.Join(hostRoot, "kit", "platform", ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "roles", "source": "./plugins/roles"}]}`)
	// A path into the repository itself means the session's clone, not the original beside the others.
	writeFile(t, filepath.Join(repo, "own", ".claude-plugin", "marketplace.json"), `{"plugins": [{"name": "a", "source": "./a"}]}`)

	got, err := declaredPlugins(repo, hostRoot, "/workspace")
	if err != nil {
		t.Fatalf("declaredPlugins() error = %v", err)
	}
	wantBeside := mountedDir{Host: filepath.Join(hostRoot, "kit", "platform"), Container: "/workspace/kit/platform"}
	if len(got) != 2 || got[1] != (pluginDir{Path: "own/a"}) ||
		got[0].Path != "/workspace/kit/platform/plugins/roles" || got[0].Beside == nil || *got[0].Beside != wantBeside {
		t.Errorf("declaredPlugins() = %+v, want own/a and the roles plugin beside the repository", got)
	}
}

func TestDeclaredPlugins_Errors(t *testing.T) {
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
			if _, err := declaredPlugins(repo, filepath.Dir(repo), "/workspace"); err == nil {
				t.Fatal("declaredPlugins() error = nil, want error")
			}
		})
	}
}

func TestDeclaredPlugins_NoSettings(t *testing.T) {
	got, err := declaredPlugins(t.TempDir(), "/src", "/workspace")
	if err != nil || got != nil {
		t.Errorf("declaredPlugins() = %+v, %v; want nothing", got, err)
	}
}

func TestDeclaredPlugins_ThisRepository(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := declaredPlugins(repo, "/src", "/workspace")
	if err != nil {
		t.Fatalf("declaredPlugins() error = %v", err)
	}
	if !slices.Contains(got, pluginDir{Path: "platform/plugins/roles"}) {
		t.Errorf("declaredPlugins() = %+v, want the roles plugin from the session's own clone", got)
	}
}
