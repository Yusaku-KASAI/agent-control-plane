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

func TestDeclaredPlugins_BesideThroughSymlink(t *testing.T) {
	hostRoot := t.TempDir()
	writeFile(t, filepath.Join(hostRoot, "kit", "platform", ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "roles", "source": "./plugins/roles"}]}`)
	// Stands in for a marketplace not yet split out of the repository that holds it.
	if err := os.Symlink(filepath.Join("kit", "platform"), filepath.Join(hostRoot, "platform")); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(hostRoot, "app")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), `{
  "extraKnownMarketplaces": {"shared": {"source": {"source": "directory", "path": "/workspace/platform"}}},
  "enabledPlugins": {"roles@shared": true}
}`)

	got, err := declaredPlugins(repo, hostRoot, "/workspace")
	if err != nil || len(got) != 1 || got[0].Path != "/workspace/platform/plugins/roles" {
		t.Errorf("declaredPlugins() = %+v, %v", got, err)
	}
}

func TestDeclaredPlugins_LinkBackIntoRepository(t *testing.T) {
	hostRoot := t.TempDir()
	writeFile(t, filepath.Join(hostRoot, "app", "kit", ".claude-plugin", "marketplace.json"), `{"plugins": []}`)
	if err := os.Symlink(filepath.Join("app", "kit"), filepath.Join(hostRoot, "kit")); err != nil {
		t.Fatal(err)
	}
	// The session's clone, which carries its own version of the marketplace.
	repo := filepath.Join(t.TempDir(), "app")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), `{
  "extraKnownMarketplaces": {"shared": {"source": {"source": "directory", "path": "/workspace/kit"}}},
  "enabledPlugins": {"roles@shared": true}
}`)
	writeFile(t, filepath.Join(repo, "kit", ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "roles", "source": "./plugins/roles"}]}`)

	got, err := declaredPlugins(repo, hostRoot, "/workspace")
	if err != nil || !slices.Equal(got, []pluginDir{{Path: "kit/plugins/roles"}}) {
		t.Errorf("declaredPlugins() = %+v, %v; want the plugin from the session's clone", got, err)
	}
}

func TestDeclaredPlugins_ThisRepository(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// The host as docs/platform/README.md sets it up: agent-platform beside the repository links into it.
	hostRoot := t.TempDir()
	for name, target := range map[string]string{filepath.Base(repo): repo, "agent-platform": filepath.Join(repo, "platform")} {
		if err := os.Symlink(target, filepath.Join(hostRoot, name)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := declaredPlugins(repo, hostRoot, "/workspace")
	if err != nil {
		t.Fatalf("declaredPlugins() error = %v", err)
	}
	if !slices.Contains(got, pluginDir{Path: "platform/plugins/roles"}) {
		t.Errorf("declaredPlugins() = %+v, want the roles plugin from the session's own clone", got)
	}
}
