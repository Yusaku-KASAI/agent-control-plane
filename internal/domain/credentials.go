package domain

// Credentials are issued per turn and handed to the agent's container. Zero values mean
// the agent has no such access.
type Credentials struct {
	GitHubToken   string
	SlackBotToken string
	// Claude Code needs one of these; an OAuth token comes from `claude setup-token`.
	ClaudeOAuthToken string
	ClaudeAPIKey     string
}
