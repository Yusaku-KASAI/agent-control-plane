package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type Config struct {
	Agents *Registry
	Claude Claude
	// SessionsRoot is the host directory holding each session's record and workspace.
	SessionsRoot string
	// IdleTimeout is how long a session's agent process waits for the next prompt.
	IdleTimeout time.Duration
	Credentials Credentials
	// GitHubApps holds the GitHub App of each agent that has one.
	GitHubApps map[domain.AgentName]GitHubApp
	// SlackApps holds the Slack app of each agent that has one.
	SlackApps map[domain.AgentName]SlackApp
	Access    AccessList
}

type SlackApp struct {
	AppTokenPath string
	BotTokenPath string
}

// AccessList allows the requesters listed for each agent.
type AccessList map[domain.AgentName]map[domain.Requester]bool

func (l AccessList) Allows(agent domain.AgentName, r domain.Requester) bool {
	return l[agent][r]
}

// Credentials is where per-session credentials are written for agent containers.
type Credentials struct {
	Dir    string
	Volume string
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
		Root        string `yaml:"root"`
		IdleTimeout string `yaml:"idle_timeout"`
	} `yaml:"sessions"`
	Credentials struct {
		Dir    string `yaml:"dir"`
		Volume string `yaml:"volume"`
	} `yaml:"credentials"`
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
		Slack *struct {
			AppTokenPath string   `yaml:"app_token_path"`
			BotTokenPath string   `yaml:"bot_token_path"`
			AllowedUsers []string `yaml:"allowed_users"`
		} `yaml:"slack"`
	} `yaml:"agents"`
}

const defaultIdleTimeout = 15 * time.Minute

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
	idle := defaultIdleTimeout
	if f.Sessions.IdleTimeout != "" {
		d, err := time.ParseDuration(f.Sessions.IdleTimeout)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("sessions.idle_timeout %q is not a positive duration", f.Sessions.IdleTimeout)
		}
		idle = d
	}
	if !filepath.IsAbs(f.Credentials.Dir) {
		return Config{}, fmt.Errorf("credentials.dir %q is not absolute", f.Credentials.Dir)
	}

	agents := make(map[domain.AgentName]domain.Agent, len(f.Agents))
	apps := make(map[domain.AgentName]GitHubApp)
	slackApps := make(map[domain.AgentName]SlackApp)
	access := make(AccessList)
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
		if sl := e.Slack; sl != nil {
			if !filepath.IsAbs(sl.AppTokenPath) || !filepath.IsAbs(sl.BotTokenPath) || len(sl.AllowedUsers) == 0 {
				return Config{}, fmt.Errorf("agent %q: slack needs absolute app_token_path and bot_token_path, and allowed_users", a.Name)
			}
			slackApps[a.Name] = SlackApp{AppTokenPath: sl.AppTokenPath, BotTokenPath: sl.BotTokenPath}
			access[a.Name] = make(map[domain.Requester]bool, len(sl.AllowedUsers))
			for _, u := range sl.AllowedUsers {
				access[a.Name][domain.Requester{Platform: "slack", ID: u}] = true
			}
		}
		agents[a.Name] = a
	}
	return Config{
		Agents:       &Registry{agents: agents},
		Claude:       Claude{ConfigVolume: f.Claude.ConfigVolume},
		SessionsRoot: f.Sessions.Root,
		IdleTimeout:  idle,
		Credentials:  Credentials{Dir: f.Credentials.Dir, Volume: f.Credentials.Volume},
		GitHubApps:   apps,
		SlackApps:    slackApps,
		Access:       access,
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
