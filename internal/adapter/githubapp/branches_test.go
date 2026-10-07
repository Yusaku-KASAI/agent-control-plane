package githubapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const sessionID domain.SessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestBranches_Remove(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		token       string
		openPRFor   string
		wantErr     error
		wantDeleted []string
	}{
		{
			name: "deletes the session branch and those named after it, skipping one already gone", origin: "git@github.com:o/repo.git", token: "t",
			wantDeleted: []string{"acp/0f8fad5b"},
		},
		{
			name: "keeps all while one has an open pull request", origin: "https://github.com/o/repo", token: "t",
			openPRFor: "o:acp/0f8fad5b-docs", wantErr: usecase.ErrBranchInReview,
		},
		{name: "without a token", origin: "https://github.com/o/repo.git"},
		{name: "without a GitHub origin", origin: "/srv/git/repo.git", token: "t"},
		{name: "without an origin", token: "t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deleted []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer t" {
					t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/repos/o/repo/git/matching-refs/heads/acp/0f8fad5b":
					_, _ = w.Write([]byte(`[{"ref":"refs/heads/acp/0f8fad5b"},{"ref":"refs/heads/acp/0f8fad5b-docs"},{"ref":"refs/heads/acp/0f8fad5bx"}]`))
				case r.Method == http.MethodGet && r.URL.Path == "/repos/o/repo/pulls":
					if r.URL.Query().Get("state") != "open" {
						t.Errorf("pulls query = %v", r.URL.Query())
					}
					if r.URL.Query().Get("head") == tt.openPRFor {
						_, _ = w.Write([]byte(`[{"number":1,"title":"Docs","html_url":"https://github.com/o/repo/pull/1"}]`))
						return
					}
					_, _ = w.Write([]byte(`[]`))
				case r.Method == http.MethodDelete:
					name := r.URL.Path[len("/repos/o/repo/git/refs/heads/"):]
					if name == "acp/0f8fad5b-docs" {
						// Deleted meanwhile, as by a merge.
						w.WriteHeader(http.StatusUnprocessableEntity)
						return
					}
					deleted = append(deleted, name)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			origin := func(context.Context, string) (string, error) { return tt.origin, nil }
			b := NewBranches(srv.URL, srv.Client(), origin)

			creds := domain.Credentials{GitHubToken: tt.token}
			pulls, err := b.OpenPullRequests(context.Background(), dev, sessionID, creds)
			wantPulls := []usecase.PullRequest(nil)
			if tt.openPRFor != "" {
				wantPulls = []usecase.PullRequest{{Number: 1, Title: "Docs", URL: "https://github.com/o/repo/pull/1"}}
			}
			if err != nil || !slices.Equal(pulls, wantPulls) {
				t.Fatalf("OpenPullRequests() = %v, %v; want %v", pulls, err, wantPulls)
			}
			removed, err := b.Remove(context.Background(), dev, sessionID, creds)
			if (tt.wantErr == nil && err != nil) || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("Remove() error = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(deleted, tt.wantDeleted) || !slices.Equal(removed, tt.wantDeleted) {
				t.Errorf("deleted = %v, Remove() = %v, want %v", deleted, removed, tt.wantDeleted)
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
