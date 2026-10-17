package filesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// Store keeps each session as <root>/<id>/session.json.
type Store struct {
	root string
	// flock locks a file; replaced in tests to stand for a filesystem where it does not exclude.
	flock func(fd int, how int) error

	mu sync.Mutex
	// held is who holds each session through this Store, which acp serve makes once and shares.
	// flock alone does not exclude on some bind mounts (Docker Desktop's fakeowner), even between
	// descriptors of one process.
	held map[domain.SessionID]*holder
}

// holder tells holders apart by address; it is not empty, as pointers to zero-size values may be equal.
type holder struct{ _ byte }

func NewStore(root string) *Store {
	return &Store{root: root, flock: syscall.Flock, held: map[domain.SessionID]*holder{}}
}

type record struct {
	ID             string        `json:"id"`
	Agent          string        `json:"agent"`
	WorkspacePath  string        `json:"workspace_path"`
	Conversation   *conversation `json:"conversation,omitempty"`
	Turns          int           `json:"turns"`
	LastUsedAt     *time.Time    `json:"last_used_at,omitempty"`
	KeptNotifiedAt *time.Time    `json:"kept_notified_at,omitempty"`
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
	if r.KeptNotifiedAt != nil {
		sess.KeptNotifiedAt = *r.KeptNotifiedAt
	}
	if c := r.Conversation; c != nil {
		sess.Conversation = domain.Conversation{Platform: c.Platform, Workspace: c.Workspace, Channel: c.Channel, Thread: c.Thread}
	}
	return sess, nil
}

func (s *Store) Save(_ context.Context, sess domain.Session) error {
	r := record{
		ID:             string(sess.ID),
		Agent:          string(sess.AgentName),
		WorkspacePath:  sess.Workspace.Path,
		Turns:          sess.Turns,
		LastUsedAt:     optionalTime(sess.LastUsedAt),
		KeptNotifiedAt: optionalTime(sess.KeptNotifiedAt),
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

// Lock is held in this Store and with flock: the first excludes among the users of this Store wherever
// the sessions are, the second excludes other Stores and processes where flock works, and neither outlives a
// crashed process. It creates the session's directory, so that the session can be locked before it
// is created. Remove moves the directory away under the lock and lets the session go, so a lock
// taken on the moved file is dropped and taken again, and the old holder no longer holds the new one.
func (s *Store) Lock(_ context.Context, id domain.SessionID) (func(), error) {
	h, ok := s.hold(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionBusy, id)
	}
	for {
		unlock, current, err := s.lockOnce(id)
		if err != nil {
			s.release(id, h)
			return nil, err
		}
		if current {
			return func() {
				unlock()
				s.release(id, h)
			}, nil
		}
		unlock()
	}
}

func (s *Store) hold(id domain.SessionID) (*holder, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[id] != nil {
		return nil, false
	}
	h := &holder{}
	s.held[id] = h
	return h, true
}

// release lets id go only if h still holds it: after Remove, a new holder may hold it.
func (s *Store) release(id domain.SessionID, h *holder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[id] == h {
		delete(s.held, id)
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
	if err := s.flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, fmt.Errorf("%w: %s", domain.ErrSessionBusy, id)
		}
		return nil, false, err
	}
	unlock = func() {
		_ = s.flock(int(f.Fd()), syscall.LOCK_UN)
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
	if err == nil {
		// The session is gone; whoever held it no longer holds a new one with the same ID.
		s.mu.Lock()
		delete(s.held, id)
		s.mu.Unlock()
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
