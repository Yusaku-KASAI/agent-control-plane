package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func TestBranches_Inspect(t *testing.T) {
	tips := map[string]string{"review": "r1", "base": "b1", "closed": "c1", "pushed-after-close": "p2", "merged": "m1", "wip": "w1", "orphan": "o1", "develop": "d1"}
	heads := map[string]string{
		"review":             `[{"number":1,"title":"Docs","html_url":"https://github.com/o/repo/pull/1","state":"open","head":{"sha":"r1"}}]`,
		"closed":             `[{"number":2,"state":"closed","head":{"sha":"c1"}}]`,
		"pushed-after-close": `[{"number":3,"state":"closed","head":{"sha":"p1"}}]`,
		"squashed":           `[{"number":4,"state":"closed","head":{"sha":"s1"}}]`,
		"squashed-after-fix": `[{"number":6,"state":"closed","head":{"sha":"f2"}}]`,
	}
	bases := map[string]string{
		"base":    `[{"number":5,"title":"Stacked","html_url":"https://github.com/o/repo/pull/5","state":"open"}]`,
		"develop": `[{"number":8,"state":"open"}]`,
	}
	// ahead tells how far head is ahead of base by "base...head"; a missing pair shares no history.
	ahead := map[string]int{"main...p2": 1, "main...m1": 0, "main...w1": 2, "main...l1": 3, "p1...p2": 1, "f2...f1": 0, "main...f1": 1}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		path := strings.TrimPrefix(r.URL.Path, "/repos/o/repo")
		q := r.URL.Query()
		switch {
		case path == "":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case strings.HasPrefix(path, "/git/ref/heads/"):
			tip, ok := tips[strings.TrimPrefix(path, "/git/ref/heads/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"object":{"sha":%q}}`, tip)
		case path == "/pulls" && q.Get("head") != "" && q.Get("state") == "all":
			if p, ok := heads[strings.TrimPrefix(q.Get("head"), "o:")]; ok {
				_, _ = w.Write([]byte(p))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case path == "/pulls" && q.Get("base") != "" && q.Get("state") == "open":
			if q.Get("base") == "develop" {
				t.Error("looked for pull requests into a branch the session did not create")
			}
			if p, ok := bases[q.Get("base")]; ok {
				_, _ = w.Write([]byte(p))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case strings.HasPrefix(path, "/compare/"):
			n, ok := ahead[strings.TrimPrefix(path, "/compare/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"ahead_by":%d}`, n)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	b := NewBranches(srv.URL, srv.Client(), func(context.Context, string) (string, error) { return "git@github.com:o/repo.git", nil })

	ref := func(name, localTip string) usecase.BranchRef {
		return usecase.BranchRef{Name: name, LocalTip: localTip, Owned: name != "develop"}
	}
	got, err := b.Inspect(context.Background(), dev, domain.Credentials{GitHubToken: "t"}, []usecase.BranchRef{
		ref("main", "x"), ref("review", "x"), ref("base", "x"), ref("closed", "x"), ref("pushed-after-close", "x"),
		ref("merged", "x"), ref("wip", "x"), ref("orphan", "x"),
		ref("squashed", "s1"), ref("squashed-after-fix", "f1"), ref("deleted-unmerged", "l1"), ref("never-pushed", "n1"),
		ref("develop", "x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []usecase.RemoteBranch{
		{BranchRef: ref("review", "x"), State: usecase.BranchInReview, Exists: true, Pulls: []usecase.PullRequest{{Number: 1, Title: "Docs", URL: "https://github.com/o/repo/pull/1"}}},
		{BranchRef: ref("base", "x"), State: usecase.BranchInReview, Exists: true, Pulls: []usecase.PullRequest{{Number: 5, Title: "Stacked", URL: "https://github.com/o/repo/pull/5"}}},
		{BranchRef: ref("closed", "x"), State: usecase.BranchDone, Exists: true},
		{BranchRef: ref("pushed-after-close", "x"), State: usecase.BranchUnfinished, Exists: true},
		{BranchRef: ref("merged", "x"), State: usecase.BranchDone, Exists: true},
		{BranchRef: ref("wip", "x"), State: usecase.BranchUnfinished, Exists: true},
		// No history in common with the default branch.
		{BranchRef: ref("orphan", "x"), State: usecase.BranchUnfinished, Exists: true},
		// Gone from the remote: judged by the local tip.
		{BranchRef: ref("squashed", "s1"), State: usecase.BranchDone},
		// The pull request got another commit after the session last pulled.
		{BranchRef: ref("squashed-after-fix", "f1"), State: usecase.BranchDone},
		{BranchRef: ref("deleted-unmerged", "l1"), State: usecase.BranchUnfinished},
		{BranchRef: ref("never-pushed", "n1"), State: usecase.BranchUnfinished},
		// Not the session's: neither a pull request into it nor its own commits keep the session.
		{BranchRef: ref("develop", "x"), State: usecase.BranchDone, Exists: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Inspect() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestBranches_Remove(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, "/repos/o/repo/git/refs/heads/")
		if r.Method != http.MethodDelete || !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		if name == "acp/0f8fad5b-docs" {
			// Deleted meanwhile, as by a merge.
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		deleted = append(deleted, name)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	b := NewBranches(srv.URL, srv.Client(), func(context.Context, string) (string, error) { return "https://github.com/o/repo", nil })

	removed, err := b.Remove(context.Background(), dev, domain.Credentials{GitHubToken: "t"}, []string{"acp/0f8fad5b", "acp/0f8fad5b-docs"})
	if want := []string{"acp/0f8fad5b"}; err != nil || !slices.Equal(removed, want) || !slices.Equal(deleted, want) {
		t.Errorf("Remove() = %v, %v, deleted %v; want %v", removed, err, deleted, want)
	}
}

func TestBranches_CouldNotHavePushed(t *testing.T) {
	for name, tt := range map[string]struct{ origin, token string }{
		"without a token":         {origin: "https://github.com/o/repo.git"},
		"without a GitHub origin": {origin: "/srv/git/repo.git", token: "t"},
		"without an origin":       {token: "t"},
	} {
		t.Run(name, func(t *testing.T) {
			b := NewBranches("http://127.0.0.1:0", http.DefaultClient, func(context.Context, string) (string, error) { return tt.origin, nil })
			creds := domain.Credentials{GitHubToken: tt.token}
			got, err := b.Inspect(context.Background(), dev, creds, []usecase.BranchRef{{Name: "acp/0f8fad5b", LocalTip: "x"}})
			if err != nil || got != nil {
				t.Errorf("Inspect() = %v, %v; want nothing", got, err)
			}
			removed, err := b.Remove(context.Background(), dev, creds, []string{"acp/0f8fad5b"})
			if err != nil || removed != nil {
				t.Errorf("Remove() = %v, %v; want nothing", removed, err)
			}
		})
	}
}

func TestParseRepo(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/o/repo.git":               "o/repo",
		"https://github.com/o/repo":                   "o/repo",
		"https://x-access-token:t@github.com/o/repo/": "o/repo",
		"git@github.com:o/repo.git":                   "o/repo",
		"ssh://git@github.com/o/repo.git":             "o/repo",
		"https://gitlab.com/o/repo.git":               "",
		"":                                            "",
	} {
		got, ok := parseRepo(remote)
		if got != want || ok != (want != "") {
			t.Errorf("parseRepo(%q) = %q, %v; want %q", remote, got, ok, want)
		}
	}
}
