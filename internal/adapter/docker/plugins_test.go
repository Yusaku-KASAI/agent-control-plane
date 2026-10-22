package docker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
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
	// The host as template/docs/guides/devcontainer.md sets it up: agent-platform beside the repository links into it.
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

// The workspace is the agent's to write: what it holds may not make the control plane read outside
// it, hang, or read without end.
func TestDeclaredPlugins_UntrustedFiles(t *testing.T) {
	const settings = `{"extraKnownMarketplaces": {"m": {"source": {"source": "directory", "path": "./p"}}}, "enabledPlugins": {"x@m": true}}`
	const catalog = `{"plugins": [{"name": "x", "source": "./x"}]}`
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "settings.json"), settings)
	writeFile(t, filepath.Join(outside, ".claude-plugin", "marketplace.json"), catalog)

	inside := func(t *testing.T, repo string) {
		writeFile(t, filepath.Join(repo, "p", ".claude-plugin", "marketplace.json"), catalog)
	}
	// Each but the last has the marketplace inside, so that read as if trusted, the plugin is found.
	tests := map[string]func(t *testing.T, repo string){
		"settings linked outside": func(t *testing.T, repo string) {
			inside(t, repo)
			symlink(t, filepath.Join(outside, "settings.json"), filepath.Join(repo, ".claude", "settings.json"))
		},
		"settings linked to a device": func(t *testing.T, repo string) {
			inside(t, repo)
			symlink(t, "/dev/zero", filepath.Join(repo, ".claude", "settings.json"))
		},
		"settings a named pipe": func(t *testing.T, repo string) {
			inside(t, repo)
			if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(filepath.Join(repo, ".claude", "settings.json"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"settings too large": func(t *testing.T, repo string) {
			inside(t, repo)
			writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings+strings.Repeat(" ", maxConfigFile))
		},
		"marketplace linked outside": func(t *testing.T, repo string) {
			writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings)
			symlink(t, outside, filepath.Join(repo, "p"))
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			arrange(t, repo)
			done := make(chan error, 1)
			go func() {
				_, err := declaredPlugins(repo, filepath.Dir(repo), "/workspace")
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Error("declaredPlugins() error = nil, want the file refused")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("declaredPlugins() hangs")
			}
		})
	}

	// A link that stays inside the repository is followed.
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings)
	writeFile(t, filepath.Join(repo, "real", ".claude-plugin", "marketplace.json"), catalog)
	symlink(t, "real", filepath.Join(repo, "p"))
	if got, err := declaredPlugins(repo, filepath.Dir(repo), "/workspace"); err != nil || !slices.Equal(got, []pluginDir{{Path: "p/x"}}) {
		t.Errorf("declaredPlugins() = %+v, %v; want the plugin through the link inside", got, err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// Between telling a regular file and opening it, the agent may swap it.
func TestOpenRegular_Swapped(t *testing.T) {
	// Each makes the replacement beside the file, while the file still holds its inode.
	tests := map[string]func(t *testing.T, next string){
		"by a named pipe": func(t *testing.T, next string) {
			if err := syscall.Mkfifo(next, 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"by another file": func(t *testing.T, next string) {
			writeFile(t, next, "other")
		},
		"by a link to a regular file": func(t *testing.T, next string) {
			writeFile(t, filepath.Join(filepath.Dir(next), "real"), "real")
			symlink(t, "real", next)
		},
	}
	for name, replace := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			writeFile(t, path, "{}")
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			info, err := root.Lstat("settings.json")
			if err != nil {
				t.Fatal(err)
			}
			next := filepath.Join(dir, "next")
			replace(t, next)
			if err := os.Rename(next, path); err != nil {
				t.Fatal(err)
			}

			done := make(chan error, 1)
			go func() {
				f, err := openRegular(root, "settings.json", info)
				if f != nil {
					_ = f.Close()
				}
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Error("openRegular() error = nil, want the swap refused")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("openRegular() hangs")
			}
		})
	}
}
