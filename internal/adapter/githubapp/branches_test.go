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

func TestBranches_InspectAndList(t *testing.T) {
	tips := map[string]string{"review": "r1", "base": "b1", "acp/0f8fad5b": "a1", "acp/0f8fad5b-docs": "d1"}
	heads := map[string]string{
		"review": `[{"number":1,"title":"Docs","html_url":"https://github.com/o/repo/pull/1","state":"open","head":{"sha":"r1"}}]`,
		"gone": `[{"number":2,"state":"closed","merged_at":"2026-10-01T00:00:00Z","head":{"sha":"m2"}},` +
			`{"number":3,"state":"closed","merged_at":null,"head":{"sha":"c3"}}]`,
	}
	bases := map[string]string{"base": `[{"number":5,"title":"Stacked","html_url":"https://github.com/o/repo/pull/5","state":"open"}]`}
	// ahead tells how far head is ahead of base by "base...head"; a missing pair shares no history.
	ahead := map[string]int{"main...m1": 0, "main...x1": 1, "m2...m1": 0, "c3...m1": 1, "m2...c1": 1, "c3...c1": 0, "main...c1": 2, "main...n1": 3, "m2...n1": 1, "c3...n1": 1,
		"main...r1": 1, "main...b1": 0, "main...a1": 0, "main...d1": 1}
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		path := strings.TrimPrefix(r.URL.Path, "/repos/o/repo")
		calls = append(calls, path+"?"+r.URL.RawQuery)
		q := r.URL.Query()
		switch {
		case path == "":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case path == "/git/matching-refs/heads/acp/0f8fad5b":
			_, _ = w.Write([]byte(`[{"ref":"refs/heads/acp/0f8fad5b"},{"ref":"refs/heads/acp/0f8fad5b-docs"}]`))
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
	creds := domain.Credentials{GitHubToken: "t"}

	names, err := b.List(context.Background(), dev, creds, "acp/0f8fad5b")
	if want := []string{"acp/0f8fad5b", "acp/0f8fad5b-docs"}; err != nil || !slices.Equal(names, want) {
		t.Errorf("List() = %v, %v; want %v", names, err, want)
	}

	got, err := b.Inspect(context.Background(), dev, creds, []usecase.BranchQuery{
		{Name: "review", Commits: []string{"x1"}},
		{Name: "base"},
		{Name: "gone", Commits: []string{"m1", "c1", "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pr := func(n int, title string) usecase.PullRequest {
		return usecase.PullRequest{Number: n, Title: title, URL: fmt.Sprintf("https://github.com/o/repo/pull/%d", n)}
	}
	want := []usecase.BranchFacts{
		{Name: "review", Exists: true, Tip: "r1", OpenHead: []usecase.PullRequest{pr(1, "Docs")},
			Commits: map[string]usecase.CommitPlaces{
				// x1 shares no history with the branch.
				"x1": {},
				"r1": {InBranch: true},
			}},
		{Name: "base", Exists: true, Tip: "b1", OpenBase: []usecase.PullRequest{pr(5, "Stacked")},
			Commits: map[string]usecase.CommitPlaces{"b1": {InDefault: true, InBranch: true}}},
		{Name: "gone", Commits: map[string]usecase.CommitPlaces{
			"m1": {InDefault: true, InMergedPull: true},
			// In the head of a pull request closed without merging, after the session last pulled.
			"c1": {InClosedPull: true},
			"n1": {},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Inspect() =\n%+v\nwant\n%+v", got, want)
	}
	if slices.Contains(calls, "/pulls?base=gone&per_page=100&state=open") {
		t.Error("asked for pull requests into a branch that is gone")
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
			got, err := b.Inspect(context.Background(), dev, creds, []usecase.BranchQuery{{Name: "acp/0f8fad5b", Commits: []string{"x"}}})
			if err != nil || got != nil {
				t.Errorf("Inspect() = %v, %v; want nothing", got, err)
			}
			names, err := b.List(context.Background(), dev, creds, "acp/0f8fad5b")
			if err != nil || names != nil {
				t.Errorf("List() = %v, %v; want nothing", names, err)
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

func TestBranches_CompareReadsOnlyAheadBy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/repo":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case strings.HasPrefix(r.URL.Path, "/repos/o/repo/git/ref/"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/repos/o/repo/pulls":
			_, _ = w.Write([]byte(`[]`))
		case strings.HasPrefix(r.URL.Path, "/repos/o/repo/compare/"):
			// Larger than any body read whole.
			_, _ = fmt.Fprintf(w, `{"status":"ahead","ahead_by":2,"files":[{"patch":%q}]}`, strings.Repeat("x", 2<<20))
		default:
			t.Errorf("unexpected %s", r.URL)
		}
	}))
	defer srv.Close()
	b := NewBranches(srv.URL, srv.Client(), func(context.Context, string) (string, error) { return "https://github.com/o/repo", nil })
	got, err := b.Inspect(context.Background(), dev, domain.Credentials{GitHubToken: "t"}, []usecase.BranchQuery{{Name: "x", Commits: []string{"c"}}})
	if err != nil || len(got) != 1 || got[0].Commits["c"] != (usecase.CommitPlaces{}) {
		t.Errorf("Inspect() = %+v, %v; want c kept nowhere", got, err)
	}
}
