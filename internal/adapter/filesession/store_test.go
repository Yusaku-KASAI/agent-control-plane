package filesession_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/filesession"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
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
		Conversation: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"},
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
	used := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(root, string(id), "session.json"), used, used); err != nil {
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
	slices.SortFunc(list, func(a, b usecase.StoredSession) int { return strings.Compare(string(a.ID), string(b.ID)) })
	if len(list) != 2 || list[0].ID != id || !list[0].LastUsed.Equal(used) || list[1].ID != orphan || time.Since(list[1].LastUsed) > time.Minute {
		t.Fatalf("List() = %+v", list)
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
	if _, err := store.Lock(ctx, id); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("Lock() after Remove error = %v, want ErrSessionNotFound", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("entries after Remove = %v, want the orphan and not-a-session", entries)
	}
}
