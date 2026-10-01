package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/config"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const validYAML = `
sessions:
  root: /var/acp/sessions
claude:
  config_volume: claude-config
agents:
  - name: repo-dev
    role: developer
    target:
      path: /src/repo
      image: repo-dev
`

func TestParse(t *testing.T) {
	cfg, err := config.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Claude.ConfigVolume != "claude-config" || cfg.SessionsRoot != "/var/acp/sessions" {
		t.Errorf("Parse() = %+v", cfg)
	}

	got, err := cfg.Agents.FindByName(context.Background(), "repo-dev")
	if err != nil {
		t.Fatalf("FindByName() error = %v", err)
	}
	want := domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: "/src/repo", Image: "repo-dev"}}
	if got != want {
		t.Errorf("FindByName() = %+v, want %+v", got, want)
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
agents: []
`,
		"relative sessions root": `
sessions: {root: w}
claude: {config_volume: v}
`,
		"invalid agent": `
sessions: {root: /w}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: relative, image: i}}
`,
		"sessions root inside a target": `
sessions: {root: /src/repo/sessions}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`,
		"sessions root is a target": `
sessions: {root: /src/repo}
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`,
		"duplicate agent": `
sessions: {root: /w}
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
claude: {config_volume: v}
agents:
  - {name: a, role: developer, target: {path: /src/repo, image: i}}
`
	if _, err := config.Parse([]byte(in)); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}
