package claudeauth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// FileIssuer reads the Claude Code credential from a file on every call: an OAuth token from
// `claude setup-token`, or an API key.
type FileIssuer struct {
	OAuthTokenPath string
	APIKeyPath     string
}

func (f FileIssuer) Issue(context.Context, domain.Agent) (domain.Credentials, error) {
	if f.OAuthTokenPath != "" {
		token, err := read(f.OAuthTokenPath)
		return domain.Credentials{ClaudeOAuthToken: token}, err
	}
	key, err := read(f.APIKeyPath)
	return domain.Credentials{ClaudeAPIKey: key}, err
}

func read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("claude credential: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
