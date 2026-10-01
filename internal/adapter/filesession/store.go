package filesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// Store keeps each session as <root>/<id>/session.json.
type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

type record struct {
	ID            string `json:"id"`
	Agent         string `json:"agent"`
	WorkspacePath string `json:"workspace_path"`
	Turns         int    `json:"turns"`
}

func (s *Store) Find(_ context.Context, id domain.SessionID) (domain.Session, error) {
	data, err := os.ReadFile(s.file(id, "session.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return domain.Session{}, fmt.Errorf("%w: %s", domain.ErrSessionNotFound, id)
	}
	if err != nil {
		return domain.Session{}, err
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return domain.Session{}, fmt.Errorf("session %s: %w", id, err)
	}
	return domain.Session{
		ID:        domain.SessionID(r.ID),
		AgentName: domain.AgentName(r.Agent),
		Workspace: domain.Workspace{Path: r.WorkspacePath},
		Turns:     r.Turns,
	}, nil
}

func (s *Store) Save(_ context.Context, sess domain.Session) error {
	data, err := json.MarshalIndent(record{
		ID:            string(sess.ID),
		Agent:         string(sess.AgentName),
		WorkspacePath: sess.Workspace.Path,
		Turns:         sess.Turns,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.file(sess.ID), 0o755); err != nil {
		return err
	}
	tmp := s.file(sess.ID, "session.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.file(sess.ID, "session.json"))
}

// Lock uses flock, so a crashed process never leaves the session locked.
func (s *Store) Lock(_ context.Context, id domain.SessionID) (func(), error) {
	f, err := os.OpenFile(s.file(id, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", domain.ErrSessionBusy, id)
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func (s *Store) file(id domain.SessionID, name ...string) string {
	return filepath.Join(append([]string{s.root, string(id)}, name...)...)
}
