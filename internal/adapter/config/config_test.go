package config_test

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const validYAML = `
sessions:
  root: /var/acp/sessions
  idle_timeout: 5m
credentials:
  dir: /var/lib/acp/credentials
  volume: acp-credentials
claude:
  config_volume: claude-config
agents:
  - name: repo-dev
    role: developer
    target:
      path: /src/repo
      image: repo-dev
    git:
      author_name: repo-dev (agent)
      author_email: 1+someone@users.noreply.github.com
    github:
      app_id: 11
      installation_id: 22
      private_key_path: /keys/repo-dev.pem
    slack:
      app_token_path: /keys/repo-dev.slack-app-token
      bot_token_path: /keys/repo-dev.slack-bot-token
      allowed_users: [U_ALICE]
  - name: repo-pm
    role: pm
    target:
      path: /src/repo
      image: repo-dev
`

func TestParse(t *testing.T) {
	cfg, err := config.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Claude.ConfigVolume != "claude-config" || cfg.SessionsRoot != "/var/acp/sessions" || cfg.IdleTimeout != 5*time.Minute ||
		cfg.Credentials != (config.Credentials{Dir: "/var/lib/acp/credentials", Volume: "acp-credentials"}) {
		t.Errorf("Parse() = %+v", cfg)
	}

	got, err := cfg.Agents.FindByName(context.Background(), "repo-dev")
	if err != nil {
		t.Fatalf("FindByName() error = %v", err)
	}
	want := domain.Agent{
		Name:   "repo-dev",
		Role:   "developer",
		Target: domain.Target{Path: "/src/repo", Image: "repo-dev"},
		Git:    domain.GitIdentity{Name: "repo-dev (agent)", Email: "1+someone@users.noreply.github.com"},
	}
	if got != want {
		t.Errorf("FindByName() = %+v, want %+v", got, want)
	}

	wantApps := map[domain.AgentName]config.GitHubApp{
		"repo-dev": {AppID: 11, InstallationID: 22, PrivateKeyPath: "/keys/repo-dev.pem"},
	}
	if !maps.Equal(cfg.GitHubApps, wantApps) {
		t.Errorf("GitHubApps = %+v, want %+v", cfg.GitHubApps, wantApps)
	}

	wantSlack := map[domain.AgentName]config.SlackApp{
		"repo-dev": {AppTokenPath: "/keys/repo-dev.slack-app-token", BotTokenPath: "/keys/repo-dev.slack-bot-token"},
	}
	if !maps.Equal(cfg.SlackApps, wantSlack) {
		t.Errorf("SlackApps = %+v, want %+v", cfg.SlackApps, wantSlack)
	}
	alice := domain.Requester{Platform: "slack", ID: "U_ALICE"}
	if !cfg.Access.Allows("repo-dev", alice) || cfg.Access.Allows("repo-pm", alice) ||
		cfg.Access.Allows("repo-dev", domain.Requester{Platform: "slack", ID: "U_BOB"}) {
		t.Error("Access does not match allowed_users")
	}

	if _, err := cfg.Agents.FindByName(context.Background(), "nobody"); !errors.Is(err, domain.ErrAgentNotFound) {
		t.Errorf("FindByName(unknown) error = %v, want ErrAgentNotFound", err)
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := map[string]string{
		"unknown field": validYAML + "extra: 1\n",
		"missing volume": `
sessions: {root: /w}
credentials: {dir: /c}
agents: []
`,
		"bad idle timeout": `
sessions: {root: /w, idle_timeout: soon}
credentials: {dir: /c}
claude: {config_volume: v}
`,
		"missing credentials dir": `
sessions: {root: /w}
claude: {config_volume: v}
`,
		"relative sessions root": `
sessions: {root: w}
claude: {config_volume: v}
`,
		"invalid agent": `
sessions: {root: /w}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: relative, image: i}}
`,
		"sessions root inside a target": `
sessions: {root: /src/repo/sessions}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`,
		"sessions root is a target": `
sessions: {root: /src/repo}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`,
		"incomplete github app": `
sessions: {root: /w}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /x, image: i}, github: {app_id: 1}}
`,
		"slack without allowed users": `
sessions: {root: /w}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /x, image: i}, slack: {app_token_path: /a, bot_token_path: /b}}
`,
		"duplicate agent": `
sessions: {root: /w}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /x, image: i}}
  - {name: a, role: pm, target: {path: /x, image: i}}
`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Parse([]byte(in)); err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestParse_SessionsRootBesideTarget(t *testing.T) {
	in := `
sessions: {root: /src/repo-sessions}
credentials: {dir: /c}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`
	if _, err := config.Parse([]byte(in)); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}
