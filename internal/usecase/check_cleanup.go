package usecase

import (
	"context"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

// KeepReason is one reason the cleanup keeps a session. Kind names the kind of reason, which tells
// how to resolve it; Text tells it as the cleanup tells the thread.
type KeepReason struct {
	Kind string
	Text string
}

// CheckCleanup tells the reasons the cleanup would keep a session, its retention aside: the same
// judgement (reasons), made on the session's workspace at path. branch is the session's default
// working branch.
func CheckCleanup(ctx context.Context, workspaces SessionWorkspace, branches SessionBranches, agent domain.Agent, branch, path string, creds domain.Credentials) ([]KeepReason, error) {
	f, err := readFacts(ctx, workspaces, branches, agent, branch, path, creds)
	if err != nil {
		return nil, err
	}
	var out []KeepReason
	for _, r := range reasons(f) {
		out = append(out, KeepReason{Kind: r.key(), Text: r.text()})
	}
	return out, nil
}
