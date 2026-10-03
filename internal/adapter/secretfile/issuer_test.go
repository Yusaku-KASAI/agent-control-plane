package secretfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/secretfile"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestSlackIssuer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot-token")
	issuer := secretfile.NewSlackIssuer(map[domain.AgentName]string{"repo-dev": path})
	dev := domain.Agent{Name: "repo-dev"}

	if _, err := issuer.Issue(context.Background(), dev); err == nil {
		t.Error("Issue() with a missing file error = nil")
	}
	for _, token := range []string{"xoxb-1", "xoxb-2"} {
		if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := issuer.Issue(context.Background(), dev)
		if err != nil || got.SlackBotToken != token {
			t.Errorf("Issue() = %+v, %v; want %s", got, err, token)
		}
	}
	if got, err := issuer.Issue(context.Background(), domain.Agent{Name: "other"}); err != nil || got != (domain.Credentials{}) {
		t.Errorf("Issue(agent without slack) = %+v, %v", got, err)
	}
}
