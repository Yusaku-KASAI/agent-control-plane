package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// pluginDir is a plugin the repository enables. Path is relative to the repository, or, for a
// marketplace beside the repository, absolute in the container, with Beside set to where that
// marketplace is on the host and in the container.
type pluginDir struct {
	Path   string
	Beside *mountedDir
}

type mountedDir struct {
	Host, Container string
}

// declaredPlugins returns the plugins the repository at repo enables in .claude/settings.json from
// directory marketplaces in its own tree or in a repository beside it (see marketplaceDir). Headless
// Claude Code does not install a project's declared plugins, so they are passed with --plugin-dir.
// Plugins from other marketplaces are left to Claude Code.
func declaredPlugins(repo, hostRoot, workspaceDir string) ([]pluginDir, error) {
	var settings struct {
		ExtraKnownMarketplaces map[string]struct {
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
		} `json:"extraKnownMarketplaces"`
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := readJSON(repo, filepath.Join(".claude", "settings.json"), &settings); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var dirs []pluginDir
	for id, enabled := range settings.EnabledPlugins {
		plugin, market, ok := strings.Cut(id, "@")
		src := settings.ExtraKnownMarketplaces[market].Source
		if !enabled || !ok || src.Source != "directory" {
			continue
		}
		var dir pluginDir
		var err error
		switch inTree, beside := marketplaceDir(src.Path, filepath.Base(repo), hostRoot, workspaceDir); {
		case beside != nil:
			var rel string
			rel, err = pluginSource(beside.Host, ".", plugin)
			dir = pluginDir{Path: path.Join(beside.Container, filepath.ToSlash(rel)), Beside: beside}
		case inTree != "":
			dir.Path, err = pluginSource(repo, inTree, plugin)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("plugin %s: %w", id, err)
		}
		dirs = append(dirs, dir)
	}
	slices.SortFunc(dirs, func(a, b pluginDir) int { return strings.Compare(a.Path, b.Path) })
	return dirs, nil
}

// marketplaceDir resolves where a directory marketplace is: relative to the repository, or at a
// path as in the devcontainer, where the repository is workspaceDir/<its name> and the
// repositories beside it on the host (under hostRoot) are workspaceDir/<their names>. It returns
// the path within the repository, also for a path beside it that links back into it, or the
// marketplace beside it, or neither for any other path.
func marketplaceDir(p, repoName, hostRoot, workspaceDir string) (inTree string, beside *mountedDir) {
	if local(p) {
		return p, nil
	}
	p = path.Clean(p)
	rel, ok := strings.CutPrefix(p, workspaceDir+"/")
	if !ok || !filepath.IsLocal(rel) {
		return "", nil
	}
	if rel == repoName {
		return ".", nil
	}
	if own, ok := strings.CutPrefix(rel, repoName+"/"); ok {
		return own, nil
	}
	host := filepath.Join(hostRoot, filepath.FromSlash(rel))
	if own, ok := linkedIntoRepository(host, filepath.Join(hostRoot, repoName)); ok {
		return own, nil
	}
	return "", &mountedDir{Host: host, Container: p}
}

// linkedIntoRepository reports where in the repository at repoRoot the host path leads, when a
// symbolic link makes it stand for a part of that repository, such as a marketplace not yet
// split out of it.
func linkedIntoRepository(host, repoRoot string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(host)
	if err != nil {
		return "", false
	}
	root, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return rel, true
}

func pluginSource(repo, marketplace, plugin string) (string, error) {
	var catalog struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := readJSON(repo, filepath.Join(marketplace, ".claude-plugin", "marketplace.json"), &catalog); err != nil {
		return "", err
	}
	for _, p := range catalog.Plugins {
		if p.Name != plugin {
			continue
		}
		var rel string
		if err := json.Unmarshal(p.Source, &rel); err != nil || !local(rel) {
			return "", errors.New("only relative plugin sources are supported")
		}
		return filepath.Join(marketplace, rel), nil
	}
	return "", fmt.Errorf("not listed in %s", marketplace)
}

// local reports whether p is a relative path that stays inside its base directory.
func local(p string) bool {
	return p != "" && filepath.IsLocal(filepath.Clean(p))
}

// maxConfigFile bounds what is read of a settings or marketplace file.
const maxConfigFile = 1 << 20

func readJSON(dir, name string, v any) error {
	data, err := readUntrusted(dir, name, maxConfigFile)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// readUntrusted reads the file name under dir, which the agent may have written in the workspace.
// Whatever symbolic links say, it reads nothing outside dir; it takes only a regular file, not a
// link to one, so a named pipe or a device cannot make it hang or read without end; and it reads at
// most limit bytes.
func readUntrusted(dir, name string, limit int64) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	f, err := openRegular(root, name, info)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", name, limit)
	}
	return data, nil
}

// openRegular opens name as the regular file info told of. It may have been replaced since, by a
// named pipe, which it opens without waiting for a writer, or by a link or another file.
func openRegular(root *os.Root, name string, info fs.FileInfo) (*os.File, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if opened, err := f.Stat(); err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = f.Close()
		return nil, fmt.Errorf("%s: changed while opened", name)
	}
	return f, nil
}
