package secretfile

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// SlackIssuer hands each agent the bot token of its own Slack app, read from a file on every
// call so that a token replaced on disk is picked up.
type SlackIssuer struct {
	tokenPaths map[domain.AgentName]string
}

func NewSlackIssuer(tokenPaths map[domain.AgentName]string) *SlackIssuer {
	return &SlackIssuer{tokenPaths: tokenPaths}
}

func (i *SlackIssuer) Issue(_ context.Context, agent domain.Agent) (domain.Credentials, error) {
	path, ok := i.tokenPaths[agent.Name]
	if !ok {
		return domain.Credentials{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return domain.Credentials{}, fmt.Errorf("slack bot token: %w", err)
	}
	return domain.Credentials{SlackBotToken: strings.TrimSpace(string(b))}, nil
}
