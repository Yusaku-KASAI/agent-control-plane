package docker

import (
	"fmt"
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

	got, err := declaredPlugins(repo, repo, "/workspace")
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
	settings := `{
  "extraKnownMarketplaces": {
    "shared": {"source": {"source": "directory", "path": "/workspace/kit/platform"}},
    "self": {"source": {"source": "directory", "path": "/workspace/app/own"}},
    "escape": {"source": {"source": "directory", "path": "/workspace/../etc"}},
    "elsewhere": {"source": {"source": "directory", "path": "/opt/plugins"}}
  },
  "enabledPlugins": {"roles@shared": true, "a@self": true, "b@escape": true, "c@elsewhere": true}
}`
	// The target's checkout declares the same; only its declaration of the marketplace beside counts.
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings)
	writeFile(t, filepath.Join(hostRoot, "app", ".claude", "settings.json"), settings)
	writeFile(t, filepath.Join(hostRoot, "kit", "platform", ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "roles", "source": "./plugins/roles"}]}`)
	// A path into the repository itself means the session's clone, not the original beside the others.
	writeFile(t, filepath.Join(repo, "own", ".claude-plugin", "marketplace.json"), `{"plugins": [{"name": "a", "source": "./a"}]}`)

	got, err := declaredPlugins(repo, filepath.Join(hostRoot, "app"), "/workspace")
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
			if _, err := declaredPlugins(repo, repo, "/workspace"); err == nil {
				t.Fatal("declaredPlugins() error = nil, want error")
			}
		})
	}
}

func TestDeclaredPlugins_NoSettings(t *testing.T) {
	got, err := declaredPlugins(t.TempDir(), "/src/app", "/workspace")
	if err != nil || got != nil {
		t.Errorf("declaredPlugins() = %+v, %v; want nothing", got, err)
	}
}

func TestDeclaredPlugins_BesideThroughSymlink(t *testing.T) {
	hostRoot := t.TempDir()
	writeFile(t, filepath.Join(hostRoot, "kit", "platform", ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "roles", "source": "./plugins/roles"}]}`)
	// Stands in for a marketplace kept in another repository and linked beside the target.
	if err := os.Symlink(filepath.Join("kit", "platform"), filepath.Join(hostRoot, "platform")); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(hostRoot, "app")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), `{
  "extraKnownMarketplaces": {"shared": {"source": {"source": "directory", "path": "/workspace/platform"}}},
  "enabledPlugins": {"roles@shared": true}
}`)

	got, err := declaredPlugins(repo, repo, "/workspace")
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

	got, err := declaredPlugins(repo, filepath.Join(hostRoot, "app"), "/workspace")
	if err != nil || !slices.Equal(got, []pluginDir{{Path: "kit/plugins/roles"}}) {
		t.Errorf("declaredPlugins() = %+v, %v; want the plugin from the session's clone", got, err)
	}
}

func TestDeclaredPlugins_ThisRepository(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// The host as docs/guides/operations.md sets it up: agent-platform beside the repository links into it.
	hostRoot := t.TempDir()
	for name, target := range map[string]string{filepath.Base(repo): repo, "agent-platform": filepath.Join(repo, "platform")} {
		if err := os.Symlink(target, filepath.Join(hostRoot, name)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := declaredPlugins(repo, filepath.Join(hostRoot, filepath.Base(repo)), "/workspace")
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
		// A device inside the workspace takes privileges to make; a link to one leads outside.
		"settings linked outside to a device": func(t *testing.T, repo string) {
			inside(t, repo)
			symlink(t, "/dev/zero", filepath.Join(repo, ".claude", "settings.json"))
		},
		// Opening waits for a writer, unless done without waiting.
		"settings a named pipe nobody writes": func(t *testing.T, repo string) {
			inside(t, repo)
			mkfifo(t, filepath.Join(repo, ".claude", "settings.json"))
		},
		// Reading waits for what the agent never writes, unless only a regular file is read.
		"settings a named pipe the agent holds open": func(t *testing.T, repo string) {
			inside(t, repo)
			pipe := filepath.Join(repo, ".claude", "settings.json")
			mkfifo(t, pipe)
			writer, err := os.OpenFile(pipe, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
		},
		// os.Root takes no absolute link, though this one leads inside.
		"settings an absolute link inside": func(t *testing.T, repo string) {
			inside(t, repo)
			writeFile(t, filepath.Join(repo, "shared", "settings.json"), settings)
			symlink(t, filepath.Join(repo, "shared", "settings.json"), filepath.Join(repo, ".claude", "settings.json"))
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
				_, err := declaredPlugins(repo, repo, "/workspace")
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
	if got, err := declaredPlugins(repo, repo, "/workspace"); err != nil || !slices.Equal(got, []pluginDir{{Path: "p/x"}}) {
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

// A link to a regular file inside the repository is read.
func TestDeclaredPlugins_SettingsLinkedInside(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "shared", "settings.json"),
		`{"extraKnownMarketplaces": {"m": {"source": {"source": "directory", "path": "./p"}}}, "enabledPlugins": {"x@m": true}}`)
	writeFile(t, filepath.Join(repo, "p", ".claude-plugin", "marketplace.json"), `{"plugins": [{"name": "x", "source": "./x"}]}`)
	symlink(t, filepath.Join("..", "shared", "settings.json"), filepath.Join(repo, ".claude", "settings.json"))

	if got, err := declaredPlugins(repo, repo, "/workspace"); err != nil || !slices.Equal(got, []pluginDir{{Path: "p/x"}}) {
		t.Errorf("declaredPlugins() = %+v, %v; want the plugin", got, err)
	}
}

// The settings in the session's clone, and links there, are the agent's to write: what they name
// beside the target is neither read nor mounted. What is beside comes from the target's checkout.
func TestDeclaredPlugins_BesideFromTheTarget(t *testing.T) {
	const catalog = `{"plugins": [{"name": "x", "source": "./x"}]}`
	settings := func(p string) string {
		return `{"extraKnownMarketplaces": {"m": {"source": {"source": "directory", "path": "` + p + `"}}}, "enabledPlugins": {"x@m": true}}`
	}
	hostRoot := t.TempDir()
	target := filepath.Join(hostRoot, "app")
	sessions := filepath.Join(hostRoot, "acp-sessions")
	repo := filepath.Join(sessions, "sid", "workspace", "app")
	elsewhere := t.TempDir()
	writeFile(t, filepath.Join(elsewhere, ".claude-plugin", "marketplace.json"), catalog)
	writeFile(t, filepath.Join(sessions, "other", "workspace", "app", "p", ".claude-plugin", "marketplace.json"), catalog)
	writeFile(t, filepath.Join(hostRoot, "kit", ".claude-plugin", "marketplace.json"), catalog)
	symlink(t, elsewhere, filepath.Join(repo, "out"))
	writeFile(t, filepath.Join(target, ".claude", "settings.json"), settings("/workspace/kit"))

	for name, p := range map[string]string{
		"through a link in the workspace to outside": "/workspace/acp-sessions/sid/workspace/app/out",
		"into another session's workspace":           "/workspace/acp-sessions/other/workspace/app/p",
		"another directory beside":                   "/workspace/kit",
	} {
		t.Run(name, func(t *testing.T) {
			writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings(p))
			got, err := declaredPlugins(repo, filepath.Join(t.TempDir(), "app"), "/workspace")
			if err != nil || len(got) != 0 {
				t.Errorf("declaredPlugins() with only the clone naming %s = %+v, %v; want nothing beside", p, got, err)
			}
		})
	}

	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings("/workspace/acp-sessions/sid/workspace/app/out"))
	got, err := declaredPlugins(repo, target, "/workspace")
	want := mountedDir{Host: filepath.Join(hostRoot, "kit"), Container: "/workspace/kit"}
	if err != nil || len(got) != 1 || got[0].Path != "/workspace/kit/x" || *got[0].Beside != want {
		t.Errorf("declaredPlugins() = %+v, %v; want only the marketplace the target declares", got, err)
	}
}

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

// What the target's checkout enables in the repository's own tree does not count: the session's
// clone, which the agent may be changing, decides that.
func TestDeclaredPlugins_OwnFromTheClone(t *testing.T) {
	hostRoot := t.TempDir()
	target := filepath.Join(hostRoot, "app")
	repo := filepath.Join(t.TempDir(), "app")
	writeFile(t, filepath.Join(target, ".claude", "settings.json"),
		`{"extraKnownMarketplaces": {"self": {"source": {"source": "directory", "path": "./own"}}}, "enabledPlugins": {"a@self": true}}`)
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"),
		`{"extraKnownMarketplaces": {"self": {"source": {"source": "directory", "path": "./own"}}}, "enabledPlugins": {"a@self": false}}`)
	writeFile(t, filepath.Join(repo, "own", ".claude-plugin", "marketplace.json"), `{"plugins": [{"name": "a", "source": "./a"}]}`)

	if got, err := declaredPlugins(repo, target, "/workspace"); err != nil || len(got) != 0 {
		t.Errorf("declaredPlugins() = %+v, %v; want nothing the clone disables", got, err)
	}
}

// A marketplace is read once for one container, however many of its plugins are enabled; what is
// read in all is bounded too, should links name it by many paths.
func TestDeclaredPlugins_ReadBoundedInAll(t *testing.T) {
	var listed []string
	for i := range 10 {
		listed = append(listed, fmt.Sprintf(`{"name": "x%d", "source": "./x%d"}`, i, i))
	}
	// Under the bound for one file; ten reads of it are over the bound in all.
	catalogFile := `{"plugins": [` + strings.Join(listed, ", ") + `]}` + strings.Repeat(" ", maxConfigRead/8)
	settings := func(paths ...string) string {
		var markets, enabled []string
		for i, p := range paths {
			markets = append(markets, fmt.Sprintf(`"m%d": {"source": {"source": "directory", "path": %q}}`, i, p))
			for j := range 10 {
				enabled = append(enabled, fmt.Sprintf(`"x%d@m%d": true`, j, i))
			}
		}
		return `{"extraKnownMarketplaces": {` + strings.Join(markets, ", ") + `}, "enabledPlugins": {` + strings.Join(enabled, ", ") + `}}`
	}

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "p", ".claude-plugin", "marketplace.json"), catalogFile)
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings("./p"))
	if got, err := declaredPlugins(repo, repo, "/workspace"); err != nil || len(got) != 10 {
		t.Errorf("declaredPlugins() = %d plugins, %v; want the ten from one reading", len(got), err)
	}

	var paths []string
	for i := range 10 {
		link := fmt.Sprintf("p%d", i)
		symlink(t, "p", filepath.Join(repo, link))
		paths = append(paths, "./"+link)
	}
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), settings(paths...))
	if _, err := declaredPlugins(repo, repo, "/workspace"); err == nil || !strings.Contains(err.Error(), "for one container") {
		t.Errorf("declaredPlugins() error = %v, want the bound in all reached", err)
	}
}
