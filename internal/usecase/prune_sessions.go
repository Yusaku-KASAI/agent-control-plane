package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

type PruneSessions struct {
	Store       SessionStore
	Credentials CredentialStore
	Now         func() time.Time
}

// Run removes and returns the sessions idle for longer than maxIdle. Sessions that are running a turn are
// skipped. A session that fails to be removed does not stop the others; their errors are joined.
func (p PruneSessions) Run(ctx context.Context, maxIdle time.Duration, dryRun bool) ([]SessionInfo, error) {
	if maxIdle <= 0 {
		return nil, fmt.Errorf("max idle %s is not positive", maxIdle)
	}
	infos, err := p.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := p.Now().Add(-maxIdle)
	var pruned []SessionInfo
	var errs []error
	for _, info := range infos {
		if !info.LastActive.Before(cutoff) {
			continue
		}
		if !dryRun {
			if err := p.remove(ctx, info.ID); errors.Is(err, domain.ErrSessionBusy) {
				continue
			} else if err != nil {
				errs = append(errs, fmt.Errorf("session %s: %w", info.ID, err))
				continue
			}
		}
		pruned = append(pruned, info)
	}
	return pruned, errors.Join(errs...)
}

func (p PruneSessions) remove(ctx context.Context, id domain.SessionID) error {
	if err := p.Store.Remove(ctx, id); err != nil {
		return err
	}
	return p.Credentials.Remove(ctx, id)
}
