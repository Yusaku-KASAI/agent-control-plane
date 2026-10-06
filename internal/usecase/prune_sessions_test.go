package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type fakeStore struct {
	infos   []SessionInfo
	busy    map[domain.SessionID]bool
	removed []domain.SessionID
}

func (f *fakeStore) List(context.Context) ([]SessionInfo, error) { return f.infos, nil }

func (f *fakeStore) Remove(_ context.Context, id domain.SessionID) error {
	if f.busy[id] {
		return fmt.Errorf("%w: %s", domain.ErrSessionBusy, id)
	}
	f.removed = append(f.removed, id)
	return nil
}

func TestPruneSessions(t *testing.T) {
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	infos := []SessionInfo{
		{ID: "old", LastActive: now.Add(-20 * day)},
		{ID: "busy", LastActive: now.Add(-20 * day)},
		{ID: "recent", LastActive: now.Add(-1 * day)},
	}
	for _, dry := range []bool{false, true} {
		store := &fakeStore{infos: infos, busy: map[domain.SessionID]bool{"busy": true}}
		creds := &fakeStore{}
		p := PruneSessions{Store: store, Credentials: creds, Now: func() time.Time { return now }}
		got, err := p.Run(context.Background(), 14*day, dry)
		if err != nil {
			t.Fatal(err)
		}
		if dry {
			if len(got) != 2 || len(store.removed) != 0 {
				t.Errorf("dry run: got %v, removed %v", got, store.removed)
			}
			continue
		}
		if len(got) != 1 || got[0].ID != "old" {
			t.Errorf("got %v, want only old", got)
		}
		if len(store.removed) != 1 || len(creds.removed) != 1 {
			t.Errorf("removed %v / %v", store.removed, creds.removed)
		}
	}
	if _, err := (PruneSessions{}).Run(context.Background(), 0, true); err == nil {
		t.Error("zero max idle should fail")
	}
}
