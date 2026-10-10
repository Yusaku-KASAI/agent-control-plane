package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/mount"

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
		hosts := fmt.Sprintf("github.com:\n    oauth_token: %s\n    user: x-access-token\n    git_protocol: https\n", creds.GitHubToken)
		if err := writeAtomic(filepath.Join(c.dir(), "gh", "hosts.yml"), []byte(hosts)); err != nil {
			return err
		}
		// Without the config version, gh migrates hosts.yml to multiple accounts on every call: it
		// fails on the read-only mount and asks GitHub for a user an app token does not have.
		if err := writeAtomic(filepath.Join(c.dir(), "gh", "config.yml"), []byte("version: \"1\"\n")); err != nil {
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
