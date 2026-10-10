package filesession_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const id domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestStore_SaveFind(t *testing.T) {
	ctx := context.Background()
	store := filesession.NewStore(t.TempDir())

	if _, err := store.Find(ctx, id); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("Find(missing) error = %v, want ErrSessionNotFound", err)
	}

	want := domain.Session{
		ID: id, AgentName: "repo-dev", Workspace: domain.Workspace{Path: "/w/repo"}, Turns: 2,
		Conversation:   domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"},
		LastUsedAt:     time.Date(2026, 10, 7, 1, 2, 3, 4, time.UTC),
		KeptNotifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 4, time.UTC),
	}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got != want {
		t.Errorf("Find() = %+v, want %+v", got, want)
	}
}

func TestStore_Lock(t *testing.T) {
	ctx := context.Background()
	store := filesession.NewStore(t.TempDir())
	if err := store.Save(ctx, domain.Session{ID: id}); err != nil {
		t.Fatal(err)
	}

	unlock, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("second Lock() error = %v, want ErrSessionBusy", err)
	}
	unlock()

	unlock, err = store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() after unlock error = %v", err)
	}
	unlock()
}

func TestStore_ListRemove(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := filesession.NewStore(root)
	const orphan domain.SessionID = "11111111-1111-4111-8111-111111111111"
	if err := store.Save(ctx, domain.Session{ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, string(orphan), "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "not-a-session"), 0o755); err != nil {
		t.Fatal(err)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	slices.Sort(list)
	if want := []domain.SessionID{id, orphan}; !slices.Equal(list, want) {
		t.Fatalf("List() = %v, want %v", list, want)
	}

	unlock, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, id); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	unlock()
	if _, err := store.Find(ctx, id); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("Find() after Remove error = %v, want ErrSessionNotFound", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("entries after Remove = %v, want the orphan and not-a-session", entries)
	}
}

func TestStore_LockBeforeCreation(t *testing.T) {
	store := filesession.NewStore(filepath.Join(t.TempDir(), "sessions"))
	unlock, err := store.Lock(context.Background(), id)
	if err != nil {
		t.Fatalf("Lock() of a session not yet created error = %v", err)
	}
	unlock()
}

// A lock taken on the file Remove moved away must not count as a lock on the new session.
func TestStore_LockAfterRemoveIsFresh(t *testing.T) {
	ctx := context.Background()
	store := filesession.NewStore(t.TempDir())
	if err := store.Save(ctx, domain.Session{ID: id}); err != nil {
		t.Fatal(err)
	}
	unlock, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, id); err != nil {
		t.Fatal(err)
	}
	second, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() after Remove while the old lock is held error = %v", err)
	}
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Errorf("third Lock() error = %v, want ErrSessionBusy", err)
	}
	second()
	unlock()
}
