package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// declaredPluginDirs returns, relative to the repository root, the plugins the repository
// enables in .claude/settings.json from marketplaces it keeps in its own tree. Headless
// Claude Code does not install a project's declared plugins, so they are passed with
// --plugin-dir. Plugins from other marketplaces are left to Claude Code.
func declaredPluginDirs(repo string) ([]string, error) {
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

	var dirs []string
	for id, enabled := range settings.EnabledPlugins {
		plugin, market, ok := strings.Cut(id, "@")
		src := settings.ExtraKnownMarketplaces[market].Source
		if !enabled || !ok || src.Source != "directory" || !local(src.Path) {
			continue
		}
		dir, err := pluginSource(repo, src.Path, plugin)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: %w", id, err)
		}
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	return dirs, nil
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
