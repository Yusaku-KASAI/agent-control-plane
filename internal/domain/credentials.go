package domain

import "errors"

// ErrCredentialsRefused tells that an issuer refused to hand out credentials for what the agent is
// set up as, which no retry mends until the setup changes.
var ErrCredentialsRefused = errors.New("credentials refused")

// Credentials are issued per turn and handed to the agent's container. Zero values mean
// the agent has no such access.
type Credentials struct {
	GitHubToken   string
	SlackBotToken string
	// Claude Code needs one of these; an OAuth token comes from `claude setup-token`.
	ClaudeOAuthToken string
	ClaudeAPIKey     string
}
