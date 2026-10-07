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
)

// Store keeps each session as <root>/<id>/session.json.
type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

type record struct {
	ID               string        `json:"id"`
	Agent            string        `json:"agent"`
	WorkspacePath    string        `json:"workspace_path"`
	Conversation     *conversation `json:"conversation,omitempty"`
	Turns            int           `json:"turns"`
	LastUsedAt       *time.Time    `json:"last_used_at,omitempty"`
	ReviewNotifiedAt *time.Time    `json:"review_notified_at,omitempty"`
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
	if r.LastUsedAt != nil {
		sess.LastUsedAt = *r.LastUsedAt
	}
	if r.ReviewNotifiedAt != nil {
		sess.ReviewNotifiedAt = *r.ReviewNotifiedAt
	}
	if c := r.Conversation; c != nil {
		sess.Conversation = domain.Conversation{Platform: c.Platform, Workspace: c.Workspace, Channel: c.Channel, Thread: c.Thread}
	}
	return sess, nil
}

func (s *Store) Save(_ context.Context, sess domain.Session) error {
	r := record{
		ID:               string(sess.ID),
		Agent:            string(sess.AgentName),
		WorkspacePath:    sess.Workspace.Path,
		Turns:            sess.Turns,
		LastUsedAt:       optionalTime(sess.LastUsedAt),
		ReviewNotifiedAt: optionalTime(sess.ReviewNotifiedAt),
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

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Lock uses flock, so a crashed process never leaves the session locked. It creates the session's
// directory, so that the session can be locked before it is created. Remove moves the directory
// away under the lock, so a lock taken on the moved file is dropped and taken again.
func (s *Store) Lock(_ context.Context, id domain.SessionID) (func(), error) {
	for {
		unlock, current, err := s.lockOnce(id)
		if err != nil || current {
			return unlock, err
		}
		unlock()
	}
}

func (s *Store) lockOnce(id domain.SessionID) (unlock func(), current bool, err error) {
	if err := os.MkdirAll(s.file(id), 0o755); err != nil {
		return nil, false, err
	}
	path := s.file(id, ".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, fmt.Errorf("%w: %s", domain.ErrSessionBusy, id)
		}
		return nil, false, err
	}
	unlock = func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	locked, err := f.Stat()
	if err != nil {
		unlock()
		return nil, false, err
	}
	now, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return unlock, false, nil
	}
	if err != nil {
		unlock()
		return nil, false, err
	}
	return unlock, os.SameFile(locked, now), nil
}

func (s *Store) List(_ context.Context) ([]domain.SessionID, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []domain.SessionID
	for _, e := range entries {
		if id, err := domain.ParseSessionID(e.Name()); err == nil && e.IsDir() {
			ids = append(ids, id)
		}
	}
	return ids, nil
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
