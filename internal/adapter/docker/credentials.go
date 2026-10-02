package docker

import (
	"fmt"
	"os"
	"path/filepath"

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
	gh := filepath.Join(c.dir(), "gh")
	if err := os.MkdirAll(gh, 0o755); err != nil {
		return err
	}
	if creds.GitHubToken == "" {
		return nil
	}
	hosts := fmt.Sprintf("github.com:\n    oauth_token: %s\n    user: x-access-token\n    git_protocol: https\n", creds.GitHubToken)
	return writeAtomic(filepath.Join(gh, "hosts.yml"), []byte(hosts))
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
