package domain

// Credentials are issued per turn and handed to the agent's container. Zero values mean
// the agent has no such access.
type Credentials struct {
	GitHubToken   string
	SlackBotToken string
}
