package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type Config struct {
	Agents *Registry
	Claude Claude
	// SessionsRoot is the host directory holding each session's record and workspace.
	SessionsRoot string
	// GitHubApps holds the GitHub App of each agent that has one.
	GitHubApps map[domain.AgentName]GitHubApp
}

type GitHubApp struct {
	AppID          int64
	InstallationID int64
	PrivateKeyPath string
}

type Claude struct {
	// ConfigVolume is the Docker volume holding a logged-in Claude Code config directory.
	ConfigVolume string
}

type fileFormat struct {
	Sessions struct {
		Root string `yaml:"root"`
	} `yaml:"sessions"`
	Claude struct {
		ConfigVolume string `yaml:"config_volume"`
	} `yaml:"claude"`
	Agents []struct {
		Name   string `yaml:"name"`
		Role   string `yaml:"role"`
		Target struct {
			Path  string `yaml:"path"`
			Image string `yaml:"image"`
		} `yaml:"target"`
		Git struct {
			AuthorName  string `yaml:"author_name"`
			AuthorEmail string `yaml:"author_email"`
		} `yaml:"git"`
		GitHub *struct {
			AppID          int64  `yaml:"app_id"`
			InstallationID int64  `yaml:"installation_id"`
			PrivateKeyPath string `yaml:"private_key_path"`
		} `yaml:"github"`
	} `yaml:"agents"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func Parse(data []byte) (Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f fileFormat
	if err := dec.Decode(&f); err != nil {
		return Config{}, err
	}
	if f.Claude.ConfigVolume == "" {
		return Config{}, errors.New("claude.config_volume is empty")
	}
	if !filepath.IsAbs(f.Sessions.Root) {
		return Config{}, fmt.Errorf("sessions.root %q is not absolute", f.Sessions.Root)
	}

	agents := make(map[domain.AgentName]domain.Agent, len(f.Agents))
	apps := make(map[domain.AgentName]GitHubApp)
	for _, e := range f.Agents {
		a, err := domain.NewAgent(domain.AgentName(e.Name), domain.Role(e.Role),
			domain.Target{Path: e.Target.Path, Image: e.Target.Image},
			domain.GitIdentity{Name: e.Git.AuthorName, Email: e.Git.AuthorEmail})
		if err != nil {
			return Config{}, err
		}
		if _, dup := agents[a.Name]; dup {
			return Config{}, fmt.Errorf("agent %q is defined more than once", a.Name)
		}
		if within(f.Sessions.Root, a.Target.Path) {
			return Config{}, fmt.Errorf("sessions.root %q is inside the target of agent %q", f.Sessions.Root, a.Name)
		}
		if gh := e.GitHub; gh != nil {
			if gh.AppID <= 0 || gh.InstallationID <= 0 || !filepath.IsAbs(gh.PrivateKeyPath) {
				return Config{}, fmt.Errorf("agent %q: github needs app_id, installation_id and an absolute private_key_path", a.Name)
			}
			apps[a.Name] = GitHubApp{AppID: gh.AppID, InstallationID: gh.InstallationID, PrivateKeyPath: gh.PrivateKeyPath}
		}
		agents[a.Name] = a
	}
	return Config{
		Agents:       &Registry{agents: agents},
		Claude:       Claude{ConfigVolume: f.Claude.ConfigVolume},
		SessionsRoot: f.Sessions.Root,
		GitHubApps:   apps,
	}, nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

type Registry struct {
	agents map[domain.AgentName]domain.Agent
}

func (r *Registry) FindByName(_ context.Context, name domain.AgentName) (domain.Agent, error) {
	a, ok := r.agents[name]
	if !ok {
		return domain.Agent{}, fmt.Errorf("%w: %q", domain.ErrAgentNotFound, name)
	}
	return a, nil
}
