package usecase_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// The check from inside the workspace makes the cleanup's judgement, with the branches the session
// owns named by its default working branch.
func TestCheckCleanup(t *testing.T) {
	const id domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	pull := usecase.PullRequest{Number: 7, Title: "fix", URL: "https://example.com/7"}
	work := fakeWork{id: {
		Changes: true,
		Local:   []usecase.LocalBranch{{BranchTip: usecase.BranchTip{Name: "HEAD", Tip: "d"}, Unpushed: true}},
	}}
	branches := &fakeBranches{remote: map[string]usecase.BranchFacts{
		"acp/0f8fad5b":     {Name: "acp/0f8fad5b", Exists: true, Tip: "a", OpenHead: []usecase.PullRequest{pull}},
		"acp/0f8fad5b-doc": {Name: "acp/0f8fad5b-doc", Exists: true, Tip: "b", Commits: map[string]usecase.CommitPlaces{"b": {InBranch: true}}},
		"acp/0f8fad5bx":    {Name: "acp/0f8fad5bx", Exists: true, Tip: "c"},
	}}

	got, err := usecase.CheckCleanup(context.Background(), work, branches, domain.Agent{Name: "repo-dev"},
		"acp/0f8fad5b", workspacePath(id), domain.Credentials{GitHubToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, r := range got {
		kinds = append(kinds, r.Kind)
		if r.Text == "" {
			t.Errorf("reason %s has no text", r.Kind)
		}
	}
	slices.Sort(kinds)
	if want := []string{"changes", "detached-commits", "open-pull", "unmerged-branch"}; !slices.Equal(kinds, want) {
		t.Errorf("CheckCleanup() kinds = %v, want %v (not the branch of another session)", kinds, want)
	}

	branches.remote = nil
	work[id] = usecase.WorkspaceState{}
	if got, err := usecase.CheckCleanup(context.Background(), work, branches, domain.Agent{Name: "repo-dev"},
		"acp/0f8fad5b", workspacePath(id), domain.Credentials{GitHubToken: "token"}); err != nil || len(got) != 0 {
		t.Errorf("CheckCleanup() of a tidy session = %+v, %v; want no reasons", got, err)
	}
}
