package filesession_test

import (
	"context"
	"errors"
	"testing"

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

	want := domain.Session{ID: id, AgentName: "repo-dev", Workspace: domain.Workspace{Path: "/w/repo"}, Turns: 2}
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
