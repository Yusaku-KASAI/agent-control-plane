package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// SessionFiles removes what Start leaves for a session outside its workspace.
type SessionFiles struct {
	opts Options
}

func NewSessionFiles(opts Options) *SessionFiles {
	return &SessionFiles{opts: opts}
}

// Remove deletes the session's credentials, and every entry named after the session ID one or two
// levels inside the agent's Claude Code config directory, such as projects/<dir>/<id>.jsonl. The
// ID is a UUID, so the names belong to no other session, and the layout of Claude Code's files
// need not be known.
func (f *SessionFiles) Remove(_ context.Context, agent domain.AgentName, id domain.SessionID) error {
	if err := os.RemoveAll(sessionCredentials{root: f.opts.CredentialsDir, id: id}.dir()); err != nil {
		return err
	}
	config := filepath.Join(f.opts.ClaudeConfigDir, string(agent))
	var errs error
	for _, pattern := range []string{"*", "*/*"} {
		matches, err := filepath.Glob(filepath.Join(config, pattern, string(id)+"*"))
		if err != nil {
			return err
		}
		for _, m := range matches {
			errs = errors.Join(errs, os.RemoveAll(m))
		}
	}
	return errs
}
