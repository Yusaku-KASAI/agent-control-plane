package docker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/mount"
	"go.yaml.in/yaml/v3"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// sessionCredentials is <root>/<session id>, mounted read-only at credentialsDir in the container.
type sessionCredentials struct {
	root string
	id   domain.SessionID
}

func (c sessionCredentials) dir() string {
	return filepath.Join(c.root, string(c.id))
}

// write replaces the files atomically, so a reader in the container never sees a partial token.
func (c sessionCredentials) write(creds domain.Credentials) error {
	for _, dir := range []string{"gh", "slack", "git"} {
		if err := os.MkdirAll(filepath.Join(c.dir(), dir), 0o755); err != nil {
			return err
		}
	}
	if creds.GitHubToken != "" {
		// Without version "1", gh migrates the config on every call and fails on the read-only mount.
		if err := writeAtomic(filepath.Join(c.dir(), "gh", "config.yml"), []byte("version: \"1\"\n")); err != nil {
			return err
		}
		hosts := fmt.Sprintf("github.com:\n    users:\n        x-access-token:\n            oauth_token: %[1]s\n"+
			"    oauth_token: %[1]s\n    user: x-access-token\n    git_protocol: https\n", creds.GitHubToken)
		if err := writeAtomic(filepath.Join(c.dir(), "gh", "hosts.yml"), []byte(hosts)); err != nil {
			return err
		}
	}
	if creds.SlackBotToken != "" {
		if err := writeAtomic(filepath.Join(c.dir(), slackTokenFile), []byte(creds.SlackBotToken)); err != nil {
			return err
		}
	}
	return nil
}

const (
	slackTokenFile = "slack/bot-token"
	coAuthorsFile  = "git/co-authors"
)

// writeCoAuthors lists one "Name <email>" per line for the workspace's commit-msg hook.
func (c sessionCredentials) writeCoAuthors(people []domain.GitIdentity) error {
	var b strings.Builder
	for _, p := range people {
		fmt.Fprintf(&b, "%s <%s>\n", p.Name, p.Email)
	}
	return writeAtomic(filepath.Join(c.dir(), coAuthorsFile), []byte(b.String()))
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func credentialsMount(id domain.SessionID, opts Options) mount.Mount {
	if opts.CredentialsVolume != "" {
		return mount.Mount{
			Type: mount.TypeVolume, Source: opts.CredentialsVolume, Target: credentialsDir, ReadOnly: true,
			VolumeOptions: &mount.VolumeOptions{Subpath: string(id)},
		}
	}
	return mount.Mount{
		Type: mount.TypeBind, Source: sessionCredentials{root: opts.CredentialsDir, id: id}.dir(),
		Target: credentialsDir, ReadOnly: true,
	}
}

// ReadGitHubToken reads the token write puts into gh's config, for acp cleanup-check in the agent's
// container, or "" without a GitHub App (no config). With a config but no token, it fails: judging
// without the remote would miss what the cleanup sees there, such as open pull requests.
func ReadGitHubToken(configDir string) (string, error) {
	if configDir == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(configDir, "hosts.yml"))
	if err != nil {
		return "", fmt.Errorf("read the GitHub token: %w", err)
	}
	var hosts map[string]struct {
		OAuthToken string `yaml:"oauth_token"`
	}
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return "", fmt.Errorf("read gh hosts: %w", err)
	}
	if hosts["github.com"].OAuthToken == "" {
		return "", errors.New("read the GitHub token: none for github.com in gh's hosts")
	}
	return hosts["github.com"].OAuthToken, nil
}
