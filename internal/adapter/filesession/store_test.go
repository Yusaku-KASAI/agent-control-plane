package filesession_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		KeptReasons:    []string{"changes   0 "},
		KeptAskedBy:    &domain.Requester{Platform: "slack", ID: "U_ALICE"},
		KeptToldAt:     time.Date(2026, 10, 8, 1, 2, 4, 5, time.UTC),
		ReportedUsage: &domain.Usage{CostUSD: 1.5, Models: map[string]domain.ModelUsage{
			"claude-opus": {InputTokens: 1, OutputTokens: 2, CacheReadInputTokens: 3, CacheCreationInputTokens: 4, CostUSD: 1.5},
		}},
	}
	for _, usage := range []*domain.Usage{want.ReportedUsage, nil} {
		want.ReportedUsage = usage
		if err := store.Save(ctx, want); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		got, err := store.Find(ctx, id)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Find() = %+v, want %+v", got, want)
		}
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

// Two Stores on the same sessions stand for two processes: flock excludes between them.
func TestStore_LockAcrossStores(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first, second := filesession.NewStore(root), filesession.NewStore(root)

	unlock, err := first.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if _, err := second.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("Lock() through another Store error = %v, want ErrSessionBusy", err)
	}
	unlock()
	again, err := second.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() through another Store after unlock error = %v", err)
	}
	again()
}

// Where flock does not exclude, the lock still holds within the Store.
func TestStore_LockWithoutFlock(t *testing.T) {
	ctx := context.Background()
	store := filesession.NewStoreWithoutFlock(t.TempDir())

	unlock, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("second Lock() error = %v, want ErrSessionBusy", err)
	}
	unlock()

	again, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() after unlock error = %v", err)
	}
	// Unlocking twice must not let go of a lock someone else has taken since.
	unlock()
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Errorf("Lock() after a stale unlock error = %v, want ErrSessionBusy", err)
	}
	again()
}

// Where flock does not exclude, Remove still lets a new session be locked, and the old holder's
// unlock does not let go of the new one.
func TestStore_LockAfterRemoveWithoutFlock(t *testing.T) {
	ctx := context.Background()
	store := filesession.NewStoreWithoutFlock(t.TempDir())
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
	unlock()
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionBusy) {
		t.Errorf("Lock() after the old holder let go error = %v, want ErrSessionBusy", err)
	}
	second()
}

// A Lock that fails lets the session go, so that it does not stay busy.
func TestStore_FailedLockLetsGo(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "sessions")
	if err := os.WriteFile(root, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store := filesession.NewStore(root)
	if _, err := store.Lock(ctx, id); err == nil || errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("Lock() under a file error = %v, want a failure to make the directory", err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	unlock, err := store.Lock(ctx, id)
	if err != nil {
		t.Fatalf("Lock() after a failed Lock error = %v", err)
	}
	unlock()
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
