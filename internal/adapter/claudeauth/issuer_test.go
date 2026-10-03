package claudeauth_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/claudeauth"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestFileIssuer(t *testing.T) {
	dir := t.TempDir()
	token, key := filepath.Join(dir, "token"), filepath.Join(dir, "key")
	_ = os.WriteFile(token, []byte("sk-ant-oat-1\n"), 0o600)
	_ = os.WriteFile(key, []byte("sk-ant-api-1\n"), 0o600)

	got, err := claudeauth.FileIssuer{OAuthTokenPath: token}.Issue(context.Background(), domain.Agent{})
	if err != nil || got != (domain.Credentials{ClaudeOAuthToken: "sk-ant-oat-1"}) {
		t.Errorf("OAuth token Issue() = %+v, %v", got, err)
	}
	got, err = claudeauth.FileIssuer{APIKeyPath: key}.Issue(context.Background(), domain.Agent{})
	if err != nil || got != (domain.Credentials{ClaudeAPIKey: "sk-ant-api-1"}) {
		t.Errorf("API key Issue() = %+v, %v", got, err)
	}
	if _, err := (claudeauth.FileIssuer{OAuthTokenPath: filepath.Join(dir, "missing")}).Issue(context.Background(), domain.Agent{}); err == nil {
		t.Error("Issue() with a missing file error = nil")
	}
}
