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
	// Retention is how long an unused session is kept.
	Retention time.Duration
	// SuggestClosingAt is the size of the agent's context, in tokens, from which a finished status
	// message suggests closing the session; 0 never suggests it.
	SuggestClosingAt int64
	// StateDir is where the control plane keeps state it alone reads and writes, such as rotating tokens.
	StateDir string
	// UsageLog is the host file where what each turn spent is appended.
	UsageLog    string
	Credentials Credentials
	Tools       Tools
	// GitHubApps holds the GitHub App of each agent that has one.
	GitHubApps map[domain.AgentName]GitHubApp
	// SlackApps holds the Slack app of each agent that has one.
	SlackApps map[domain.AgentName]SlackApp
	Access    AccessList
	People    People
}

// People maps requesters to how they are credited in commits.
type People map[domain.Requester]domain.GitIdentity

func (p People) GitIdentity(r domain.Requester) (domain.GitIdentity, bool) {
	id, ok := p[r]
	return id, ok
}

type SlackApp struct {
	AppTokenPath string
	// BotTokenPath is the bot token, or with Rotation the long-lived one exchanged when rotation starts.
	BotTokenPath string
	Rotation     *SlackRotation
}

type SlackRotation struct {
	ClientID         string
	ClientSecretPath string
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

// Tools is where the control plane puts its own binary for the containers it starts to run.
type Tools struct {
	Dir    string
	Volume string
}

type GitHubApp struct {
	AppID          int64
	InstallationID int64
	PrivateKeyPath string
}

// Claude is how agents run Claude Code: one of OAuthTokenPath and APIKeyPath, and where each
// agent's config directory lives.
type Claude struct {
	OAuthTokenPath string
	APIKeyPath     string
	ConfigDir      string
	ConfigVolume   string
}

type fileFormat struct {
	Sessions struct {
		Root        string `yaml:"root"`
		IdleTimeout string `yaml:"idle_timeout"`
		Retention   string `yaml:"retention"`
		// SuggestClosingAt is a pointer to tell 0, which turns the suggestion off, from unset.
		SuggestClosingAt *int64 `yaml:"suggest_closing_at"`
	} `yaml:"sessions"`
	Credentials struct {
		Dir    string `yaml:"dir"`
		Volume string `yaml:"volume"`
	} `yaml:"credentials"`
	Tools struct {
		Dir    string `yaml:"dir"`
		Volume string `yaml:"volume"`
	} `yaml:"tools"`
	StateDir string `yaml:"state_dir"`
	UsageLog string `yaml:"usage_log"`
	People   []struct {
		Slack    string `yaml:"slack"`
		GitName  string `yaml:"git_name"`
		GitEmail string `yaml:"git_email"`
	} `yaml:"people"`
	Claude struct {
		OAuthTokenPath string `yaml:"oauth_token_path"`
		APIKeyPath     string `yaml:"api_key_path"`
		ConfigDir      string `yaml:"config_dir"`
		ConfigVolume   string `yaml:"config_volume"`
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
			Rotation     *struct {
				ClientID         string `yaml:"client_id"`
				ClientSecretPath string `yaml:"client_secret_path"`
			} `yaml:"rotation"`
		} `yaml:"slack"`
	} `yaml:"agents"`
}

const (
	defaultIdleTimeout = 15 * time.Minute
	defaultRetention   = 30 * 24 * time.Hour
	// defaultSuggestClosingAt is well below the size at which Claude Code compacted the 1M-token
	// context of a long thread (Issue #240), and above that of a session doing one piece of work.
	defaultSuggestClosingAt = 400_000
)

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
	cl := f.Claude
	if (cl.OAuthTokenPath == "") == (cl.APIKeyPath == "") {
		return Config{}, errors.New("claude needs exactly one of oauth_token_path and api_key_path")
	}
	for _, p := range []string{cl.OAuthTokenPath, cl.APIKeyPath} {
		if p != "" && !filepath.IsAbs(p) {
			return Config{}, fmt.Errorf("claude credential path %q is not absolute", p)
		}
	}
	if !filepath.IsAbs(cl.ConfigDir) {
		return Config{}, fmt.Errorf("claude.config_dir %q is not absolute", cl.ConfigDir)
	}
	if !filepath.IsAbs(f.Sessions.Root) {
		return Config{}, fmt.Errorf("sessions.root %q is not absolute", f.Sessions.Root)
	}
	if f.UsageLog == "" {
		return Config{}, errors.New("usage_log is not set")
	}
	if !filepath.IsAbs(f.UsageLog) {
		return Config{}, fmt.Errorf("usage_log %q is not absolute", f.UsageLog)
	}
	idle := defaultIdleTimeout
	if f.Sessions.IdleTimeout != "" {
		d, err := time.ParseDuration(f.Sessions.IdleTimeout)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("sessions.idle_timeout %q is not a positive duration", f.Sessions.IdleTimeout)
		}
		idle = d
	}
	retention := defaultRetention
	if f.Sessions.Retention != "" {
		d, err := time.ParseDuration(f.Sessions.Retention)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("sessions.retention %q is not a positive duration", f.Sessions.Retention)
		}
		retention = d
	}
	suggestClosingAt := int64(defaultSuggestClosingAt)
	if n := f.Sessions.SuggestClosingAt; n != nil {
		if *n < 0 {
			return Config{}, fmt.Errorf("sessions.suggest_closing_at %d is negative", *n)
		}
		suggestClosingAt = *n
	}
	if !filepath.IsAbs(f.Credentials.Dir) {
		return Config{}, fmt.Errorf("credentials.dir %q is not absolute", f.Credentials.Dir)
	}
	if !filepath.IsAbs(f.Tools.Dir) {
		return Config{}, fmt.Errorf("tools.dir %q is not absolute", f.Tools.Dir)
	}

	people := make(People, len(f.People))
	for _, p := range f.People {
		if p.Slack == "" || p.GitName == "" || p.GitEmail == "" {
			return Config{}, errors.New("people need slack, git_name and git_email")
		}
		people[domain.Requester{Platform: "slack", ID: p.Slack}] = domain.GitIdentity{Name: p.GitName, Email: p.GitEmail}
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
			app := SlackApp{AppTokenPath: sl.AppTokenPath, BotTokenPath: sl.BotTokenPath}
			if r := sl.Rotation; r != nil {
				if r.ClientID == "" || !filepath.IsAbs(r.ClientSecretPath) {
					return Config{}, fmt.Errorf("agent %q: slack.rotation needs client_id and an absolute client_secret_path", a.Name)
				}
				if !filepath.IsAbs(f.StateDir) {
					return Config{}, fmt.Errorf("agent %q: slack.rotation needs an absolute state_dir", a.Name)
				}
				app.Rotation = &SlackRotation{ClientID: r.ClientID, ClientSecretPath: r.ClientSecretPath}
			}
			slackApps[a.Name] = app
			access[a.Name] = make(map[domain.Requester]bool, len(sl.AllowedUsers))
			for _, u := range sl.AllowedUsers {
				access[a.Name][domain.Requester{Platform: "slack", ID: u}] = true
			}
		}
		agents[a.Name] = a
	}
	return Config{
		Agents:           &Registry{agents: agents},
		Claude:           Claude{OAuthTokenPath: cl.OAuthTokenPath, APIKeyPath: cl.APIKeyPath, ConfigDir: cl.ConfigDir, ConfigVolume: cl.ConfigVolume},
		SessionsRoot:     f.Sessions.Root,
		IdleTimeout:      idle,
		Retention:        retention,
		SuggestClosingAt: suggestClosingAt,
		StateDir:         f.StateDir,
		UsageLog:         f.UsageLog,
		Credentials:      Credentials{Dir: f.Credentials.Dir, Volume: f.Credentials.Volume},
		Tools:            Tools{Dir: f.Tools.Dir, Volume: f.Tools.Volume},
		GitHubApps:       apps,
		SlackApps:        slackApps,
		Access:           access,
		People:           people,
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
