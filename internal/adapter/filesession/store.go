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
	"time"

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
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotFound, id)
	}
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

// List takes when the session was last used from the record's modification time, since the
// record is saved when the session is created and after every turn. A directory without a record
// is one whose creation stopped before saving it.
func (s *Store) List(_ context.Context) ([]usecase.StoredSession, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []usecase.StoredSession
	for _, e := range entries {
		id, err := domain.ParseSessionID(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		info, err := os.Stat(s.file(id, "session.json"))
		if errors.Is(err, fs.ErrNotExist) {
			info, err = e.Info()
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		list = append(list, usecase.StoredSession{ID: id, LastUsed: info.ModTime()})
	}
	return list, nil
}

// Remove first moves the directory out of the session's path in one step, so that the session
// is gone at once and a new one with the same ID can be created while the old files are deleted.
// It also deletes what earlier calls failed to.
func (s *Store) Remove(_ context.Context, id domain.SessionID) error {
	trash := filepath.Join(s.root, fmt.Sprintf("%s%s-%d", removedPrefix, id, time.Now().UnixNano()))
	err := os.Rename(s.file(id), trash)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	removed, err := filepath.Glob(filepath.Join(s.root, removedPrefix+"*"))
	if err != nil {
		return err
	}
	var errs error
	for _, dir := range removed {
		errs = errors.Join(errs, os.RemoveAll(dir))
	}
	return errs
}

const removedPrefix = ".removed-"

func (s *Store) file(id domain.SessionID, name ...string) string {
	return filepath.Join(append([]string{s.root, string(id)}, name...)...)
}
