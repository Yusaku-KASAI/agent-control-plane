package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
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
// directory marketplaces: one in its own tree (a relative path), or one in a repository beside it
// (an absolute path under workspaceDir, which stands for hostRoot as in the devcontainer). Headless
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
	if err := readJSON(filepath.Join(repo, ".claude", "settings.json"), &settings); err != nil {
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
		switch beside, isBeside := besideRepository(src.Path, filepath.Base(repo), hostRoot, workspaceDir); {
		case local(src.Path):
			dir.Path, err = pluginSource(repo, src.Path, plugin)
		case isBeside:
			var rel string
			rel, err = pluginSource(beside.Host, ".", plugin)
			dir = pluginDir{Path: path.Join(beside.Container, filepath.ToSlash(rel)), Beside: &beside}
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

// besideRepository maps a container path under workspaceDir that is not in the repository itself
// to the same place under hostRoot.
func besideRepository(p, repoName, hostRoot, workspaceDir string) (mountedDir, bool) {
	p = path.Clean(p)
	rel, ok := strings.CutPrefix(p, workspaceDir+"/")
	if !ok || !path.IsAbs(p) || rel == repoName || strings.HasPrefix(rel, repoName+"/") || !filepath.IsLocal(rel) {
		return mountedDir{}, false
	}
	return mountedDir{Host: filepath.Join(hostRoot, filepath.FromSlash(rel)), Container: p}, true
}

func pluginSource(repo, marketplace, plugin string) (string, error) {
	var catalog struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := readJSON(filepath.Join(repo, marketplace, ".claude-plugin", "marketplace.json"), &catalog); err != nil {
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

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
