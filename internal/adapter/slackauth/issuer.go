package slackauth

import (
	"context"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// Issuer hands each agent the current bot token of its own Slack app.
type Issuer struct {
	sources map[domain.AgentName]Source
}

func NewIssuer(sources map[domain.AgentName]Source) *Issuer {
	return &Issuer{sources: sources}
}

func (i *Issuer) Issue(ctx context.Context, agent domain.Agent) (domain.Credentials, error) {
	src, ok := i.sources[agent.Name]
	if !ok {
		return domain.Credentials{}, nil
	}
	token, err := src.Token(ctx)
	if err != nil {
		return domain.Credentials{}, err
	}
	return domain.Credentials{SlackBotToken: token}, nil
}
