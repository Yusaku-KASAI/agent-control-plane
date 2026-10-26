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

type settingsFile struct {
	ExtraKnownMarketplaces map[string]struct {
		Source struct {
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"source"`
	} `json:"extraKnownMarketplaces"`
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

// declaredPlugins returns the plugins enabled in .claude/settings.json from directory marketplaces
// in the repository's own tree or in a repository beside it (see marketplaceDir). Headless Claude
// Code does not install a project's declared plugins, so they are passed with --plugin-dir. Plugins
// from other marketplaces are left to Claude Code.
//
// The repository's own marketplaces are read in the session's clone at repo, where the agent may be
// changing them, and as its settings there declare them. A marketplace beside it is the host's, and
// is mounted into the agent's container: it is taken as the target's checkout at target declares it,
// for the agent may write anything in its clone, links included.
func declaredPlugins(repo, target, workspaceDir string) ([]pluginDir, error) {
	hostRoot, name := filepath.Dir(target), filepath.Base(repo)
	reader := &configReader{left: maxConfigRead, catalogs: make(map[string]catalog)}
	var dirs []pluginDir
	for _, from := range []struct {
		dir    string
		beside bool
	}{{repo, false}, {target, true}} {
		var settings settingsFile
		if err := reader.readJSON(from.dir, filepath.Join(".claude", "settings.json"), &settings); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		for id, enabled := range settings.EnabledPlugins {
			plugin, market, ok := strings.Cut(id, "@")
			src := settings.ExtraKnownMarketplaces[market].Source
			if !enabled || !ok || src.Source != "directory" {
				continue
			}
			var dir pluginDir
			var err error
			switch inTree, beside := marketplaceDir(src.Path, name, hostRoot, workspaceDir); {
			case beside != nil && from.beside:
				var rel string
				rel, err = pluginSource(reader, beside.Host, ".", plugin)
				dir = pluginDir{Path: path.Join(beside.Container, filepath.ToSlash(rel)), Beside: beside}
			case inTree != "" && !from.beside:
				dir.Path, err = pluginSource(reader, repo, inTree, plugin)
			default:
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", id, err)
			}
			dirs = append(dirs, dir)
		}
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
// symbolic link makes it stand for a part of that repository, such as a marketplace the repository
// holds and links beside itself.
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

type catalog struct {
	Plugins []struct {
		Name   string          `json:"name"`
		Source json.RawMessage `json:"source"`
	} `json:"plugins"`
}

func pluginSource(reader *configReader, repo, marketplace, plugin string) (string, error) {
	catalog, err := reader.catalog(repo, marketplace)
	if err != nil {
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

const (
	// maxConfigFile bounds what is read of a settings or marketplace file.
	maxConfigFile = 1 << 20
	// maxConfigRead bounds what is read for one container in all, should the settings name one
	// marketplace by many paths through links.
	maxConfigRead = 4 << 20
)

var errTooLarge = errors.New("larger than allowed")

// configReader reads the settings and marketplaces for one container: each marketplace once, and
// no more than maxConfigRead in all.
type configReader struct {
	left     int64
	catalogs map[string]catalog
}

// catalog reads the marketplace at dir/marketplace, once for one container.
func (b *configReader) catalog(dir, marketplace string) (catalog, error) {
	key := filepath.Join(dir, marketplace)
	if c, ok := b.catalogs[key]; ok {
		return c, nil
	}
	var c catalog
	if err := b.readJSON(dir, filepath.Join(marketplace, ".claude-plugin", "marketplace.json"), &c); err != nil {
		return c, err
	}
	b.catalogs[key] = c
	return c, nil
}

func (b *configReader) readJSON(dir, name string, v any) error {
	limit := min(maxConfigFile, b.left)
	data, err := readUntrusted(dir, name, limit)
	b.left -= int64(len(data))
	if errors.Is(err, errTooLarge) {
		if limit < maxConfigFile {
			err = fmt.Errorf("the read limit for one container (%d bytes) reached", maxConfigRead)
		} else {
			err = fmt.Errorf("larger than %d bytes", maxConfigFile)
		}
	}
	if err == nil {
		err = json.Unmarshal(data, v)
	}
	if err != nil {
		// The same name is read in the session's clone and in the target's checkout.
		return fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
	}
	return nil
}

// readUntrusted reads the file name under dir, which the agent may have written in the workspace.
// Whatever symbolic links say, it reads nothing outside dir; it opens without waiting and takes
// only a regular file, so a named pipe or a device cannot make it hang or read without end; and it
// reads at most limit bytes.
func readUntrusted(dir, name string, limit int64) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errTooLarge
	}
	return data, nil
}
