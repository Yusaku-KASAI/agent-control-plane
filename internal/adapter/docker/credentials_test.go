package docker

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// The workspace's pre-push hook reads the token from gh's hosts as the value of oauth_token lines,
// unquoted and alone on its line (gitworkspace's hooks/pre-push.sh), and the Slack token as the
// whole of its file.
func TestWrite_TokensInTheFormThePrePushHookReads(t *testing.T) {
	c := sessionCredentials{root: t.TempDir(), id: domain.SessionID("0f8fad5b-d9cb-469f-a165-70867728950e")}
	const gh, slack = "gh-token.with_odd-chars", "slack-token"
	if err := c.write(domain.Credentials{GitHubToken: gh, SlackBotToken: slack}); err != nil {
		t.Fatal(err)
	}
	hosts, err := os.ReadFile(filepath.Join(c.dir(), "gh", "hosts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`^\s*oauth_token:\s*` + regexp.QuoteMeta(gh) + `$`)
	var found bool
	for _, l := range strings.Split(string(hosts), "\n") {
		if strings.Contains(l, gh) {
			found = true
			if !line.MatchString(l) {
				t.Errorf("hosts.yml holds the token in %q, not as the pre-push hook reads it", l)
			}
		}
	}
	if !found {
		t.Errorf("hosts.yml does not hold the token: %s", hosts)
	}
	if got, err := os.ReadFile(filepath.Join(c.dir(), slackTokenFile)); err != nil || strings.TrimSpace(string(got)) != slack {
		t.Errorf("the Slack token file = %q, %v; want only the token", got, err)
	}
}

// The token read in the agent's container is the one written for gh.
func TestReadGitHubToken(t *testing.T) {
	c := sessionCredentials{root: t.TempDir(), id: domain.SessionID("0f8fad5b-d9cb-469f-a165-70867728950e")}
	if err := c.write(domain.Credentials{GitHubToken: "ghs_tok"}); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadGitHubToken(filepath.Join(c.dir(), "gh")); err != nil || got != "ghs_tok" {
		t.Errorf("ReadGitHubToken() = %q, %v; want ghs_tok", got, err)
	}
	if got, err := ReadGitHubToken(""); err != nil || got != "" {
		t.Errorf("ReadGitHubToken() without a GitHub App = %q, %v; want none", got, err)
	}
	// With a GitHub App, judging without the token would miss what the cleanup sees on the remote.
	if got, err := ReadGitHubToken(t.TempDir()); err == nil {
		t.Errorf("ReadGitHubToken() with no hosts = %q; want an error", got)
	}
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "hosts.yml"), []byte("github.com:\n    user: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadGitHubToken(empty); err == nil {
		t.Errorf("ReadGitHubToken() with no token in hosts = %q; want an error", got)
	}
}
