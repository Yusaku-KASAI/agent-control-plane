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
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// Store keeps each session as <root>/<id>/session.json.
type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

type record struct {
	ID            string        `json:"id"`
	Agent         string        `json:"agent"`
	WorkspacePath string        `json:"workspace_path"`
	Conversation  *conversation `json:"conversation,omitempty"`
	Turns         int           `json:"turns"`
}

type conversation struct {
	Platform  string `json:"platform"`
	Workspace string `json:"workspace"`
	Channel   string `json:"channel"`
	Thread    string `json:"thread"`
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
	sess := domain.Session{
		ID:        domain.SessionID(r.ID),
		AgentName: domain.AgentName(r.Agent),
		Workspace: domain.Workspace{Path: r.WorkspacePath},
		Turns:     r.Turns,
	}
	if c := r.Conversation; c != nil {
		sess.Conversation = domain.Conversation{Platform: c.Platform, Workspace: c.Workspace, Channel: c.Channel, Thread: c.Thread}
	}
	return sess, nil
}

func (s *Store) Save(_ context.Context, sess domain.Session) error {
	r := record{
		ID:            string(sess.ID),
		Agent:         string(sess.AgentName),
		WorkspacePath: sess.Workspace.Path,
		Turns:         sess.Turns,
	}
	if c := sess.Conversation; c != (domain.Conversation{}) {
		r.Conversation = &conversation{Platform: c.Platform, Workspace: c.Workspace, Channel: c.Channel, Thread: c.Thread}
	}
	data, err := json.MarshalIndent(r, "", "  ")
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

// List skips directories that are not sessions, such as one whose record was never saved.
func (s *Store) List(_ context.Context) ([]usecase.SessionInfo, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var infos []usecase.SessionInfo
	for _, e := range entries {
		id, err := domain.ParseSessionID(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		st, err := os.Stat(s.file(id, "session.json"))
		if err != nil {
			continue
		}
		infos = append(infos, usecase.SessionInfo{ID: id, LastActive: st.ModTime()})
	}
	return infos, nil
}

// Remove holds the session's lock while deleting, so a session that starts a turn meanwhile
// finds it busy instead of losing its workspace.
func (s *Store) Remove(ctx context.Context, id domain.SessionID) error {
	unlock, err := s.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	return os.RemoveAll(s.file(id))
}
