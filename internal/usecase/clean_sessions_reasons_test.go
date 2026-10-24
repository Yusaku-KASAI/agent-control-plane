package usecase

import (
	"slices"
	"testing"
)

// TestReasons covers the axes a session is judged by: the working tree, where a local or pushed
// commit is kept, and the pull requests and content of the branches the session pushed or created.
func TestReasons(t *testing.T) {
	pr := PullRequest{Number: 1, Title: "T", URL: "u"}
	local := func(name string, unpushed bool) LocalBranch {
		return LocalBranch{BranchTip: BranchTip{Name: name, Tip: "c-" + name}, Unpushed: unpushed}
	}
	pushed := func(name string) BranchTip { return BranchTip{Name: name, Tip: "c-" + name} }
	// remote is a branch on GitHub whose tip, if it exists, is "tip-"+name.
	remote := func(name string, exists bool, places CommitPlaces) BranchFacts {
		b := BranchFacts{Name: name, Exists: exists, Commits: map[string]CommitPlaces{"c-" + name: places}}
		if exists {
			b.Tip = "tip-" + name
			b.Commits[b.Tip] = places
		}
		return b
	}
	nowhere := CommitPlaces{}
	tests := []struct {
		name      string
		facts     sessionFacts
		want      []reason
		removable []string
	}{
		{name: "nothing", facts: sessionFacts{}},
		{name: "uncommitted or untracked", facts: sessionFacts{workspace: WorkspaceState{Changes: true}}, want: []reason{{kind: reasonChanges}}},
		{name: "stash", facts: sessionFacts{workspace: WorkspaceState{Stash: true}}, want: []reason{{kind: reasonStash}}},
		{name: "operation halfway", facts: sessionFacts{workspace: WorkspaceState{Operation: "rebase"}}, want: []reason{{kind: reasonOperation, operation: "rebase"}}},
		{name: "detached HEAD on commits no branch has", facts: sessionFacts{workspace: WorkspaceState{Local: []LocalBranch{local("HEAD", true)}}},
			want: []reason{{kind: reasonLostCommits, branch: "HEAD"}}},
		{name: "local commits, remote unreadable", facts: sessionFacts{workspace: WorkspaceState{Local: []LocalBranch{local("x", true)}}},
			want: []reason{{kind: reasonLostCommits, branch: "x"}}},
		{name: "local commits already on a remote-tracking branch", facts: sessionFacts{workspace: WorkspaceState{Local: []LocalBranch{local("x", false)}}, branches: map[string]BranchFacts{}}},
		{name: "local commits nowhere on the remote", facts: sessionFacts{
			workspace: WorkspaceState{Local: []LocalBranch{local("x", true)}},
			branches:  map[string]BranchFacts{"x": remote("x", false, nowhere)},
		}, want: []reason{{kind: reasonLostCommits, branch: "x"}}},
		{name: "local commits in the default branch", facts: sessionFacts{
			workspace: WorkspaceState{Local: []LocalBranch{local("x", true)}},
			branches:  map[string]BranchFacts{"x": remote("x", false, CommitPlaces{InDefault: true})},
		}},
		{name: "local commits in a merged pull request, squashed and pruned", facts: sessionFacts{
			workspace: WorkspaceState{Local: []LocalBranch{local("x", true)}},
			branches:  map[string]BranchFacts{"x": remote("x", false, CommitPlaces{InMergedPull: true})},
		}},
		{name: "local commits in a pull request closed without merging", facts: sessionFacts{
			workspace: WorkspaceState{Local: []LocalBranch{local("x", true)}},
			branches:  map[string]BranchFacts{"x": remote("x", false, CommitPlaces{InClosedPull: true})},
		}},
		{name: "pushed to another's branch, still there", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("other")}},
			branches:  map[string]BranchFacts{"other": remote("other", true, CommitPlaces{InBranch: true})},
		}},
		{name: "pushed to another's branch, deleted without merging", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("other")}},
			branches:  map[string]BranchFacts{"other": remote("other", false, nowhere)},
		}, want: []reason{{kind: reasonLostPush, branch: "other"}}},
		{name: "pushed to another's branch with an open pull request", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("other")}},
			branches: map[string]BranchFacts{"other": func() BranchFacts {
				b := remote("other", true, CommitPlaces{InBranch: true})
				b.OpenHead = []PullRequest{pr}
				return b
			}()},
		}, want: []reason{{kind: reasonOpenPull, pull: pr}}},
		{name: "another's branch fetched only, with an open pull request", facts: sessionFacts{
			workspace: WorkspaceState{Local: []LocalBranch{local("develop", false)}},
			branches: map[string]BranchFacts{"develop": func() BranchFacts {
				b := remote("develop", true, CommitPlaces{InBranch: true})
				b.OpenHead = []PullRequest{pr}
				return b
			}()},
		}},
		{name: "pushed to another's branch, force-pushed away by someone", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("other")}},
			branches:  map[string]BranchFacts{"other": remote("other", true, nowhere)},
		}, want: []reason{{kind: reasonLostPush, branch: "other"}}},
		{name: "pushed, remote unreadable", facts: sessionFacts{workspace: WorkspaceState{Pushed: []BranchTip{pushed("other")}}}},
		{name: "several reasons, each once", facts: sessionFacts{
			workspace: WorkspaceState{Changes: true, Local: []LocalBranch{local("HEAD", true), local("HEAD", true)}},
		}, want: []reason{{kind: reasonChanges}, {kind: reasonLostCommits, branch: "HEAD"}}},
		// After others pushed it away and the workspace fetched, the pushed commit is no longer on
		// origin's remote-tracking branches: the branch at it holds only the lost push.
		{name: "pushed commit pushed away, fetched", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("x")}, Local: []LocalBranch{{BranchTip: pushed("x"), Unpushed: true}}},
			branches:  map[string]BranchFacts{"x": remote("x", true, nowhere)},
		}, want: []reason{{kind: reasonLostPush, branch: "x"}}},
		// Without the remote, the lost push is unseen; the commit stays the workspace's to keep.
		{name: "pushed commit pushed away, fetched, remote unreadable", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("x")}, Local: []LocalBranch{{BranchTip: pushed("x"), Unpushed: true}}},
		}, want: []reason{{kind: reasonLostCommits, branch: "x"}}},
		// Commits made after the push are the workspace's alone; the pushed one, others pushed away.
		{name: "pushed commit pushed away, and commits since", facts: sessionFacts{
			workspace: WorkspaceState{Pushed: []BranchTip{pushed("x")}, Local: []LocalBranch{{BranchTip: BranchTip{Name: "x", Tip: "c-x-since"}, Unpushed: true}}},
			branches:  map[string]BranchFacts{"x": {Name: "x", Commits: map[string]CommitPlaces{"c-x": {}}}},
		}, want: []reason{{kind: reasonLostCommits, branch: "x"}, {kind: reasonLostPush, branch: "x"}}},
		{name: "own branch merged", facts: sessionFacts{
			owned:    []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": remote("acp/1", true, CommitPlaces{InDefault: true, InBranch: true})},
		}, removable: []string{"acp/1"}},
		{name: "own branch in a pull request closed without merging", facts: sessionFacts{
			owned:    []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": remote("acp/1", true, CommitPlaces{InClosedPull: true, InBranch: true})},
		}, removable: []string{"acp/1"}},
		{name: "own branch kept only by itself, not in the workspace", facts: sessionFacts{
			owned:    []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": remote("acp/1", true, CommitPlaces{InBranch: true})},
		}, want: []reason{{kind: reasonUnmerged, branch: "acp/1", tip: "tip-acp/1"}}, removable: []string{"acp/1"}},
		{name: "own branch is the base of an open pull request", facts: sessionFacts{
			owned: []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": func() BranchFacts {
				b := remote("acp/1", true, CommitPlaces{InDefault: true, InBranch: true})
				b.OpenBase = []PullRequest{pr}
				return b
			}()},
		}, want: []reason{{kind: reasonOpenBase, branch: "acp/1", pull: pr}}, removable: []string{"acp/1"}},
		{name: "own branch with an open pull request", facts: sessionFacts{
			owned: []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": func() BranchFacts {
				b := remote("acp/1", true, CommitPlaces{InBranch: true})
				b.OpenHead = []PullRequest{pr}
				return b
			}()},
		}, want: []reason{{kind: reasonOpenPull, pull: pr}}, removable: []string{"acp/1"}},
		{name: "own branch gone", facts: sessionFacts{
			owned:    []string{"acp/1"},
			branches: map[string]BranchFacts{"acp/1": remote("acp/1", false, nowhere)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reasons(tt.facts); !slices.Equal(got, tt.want) {
				t.Errorf("reasons() = %+v, want %+v", got, tt.want)
			}
			if got := tt.facts.removable(); !slices.Equal(got, tt.removable) {
				t.Errorf("removable() = %v, want %v", got, tt.removable)
			}
		})
	}
}
