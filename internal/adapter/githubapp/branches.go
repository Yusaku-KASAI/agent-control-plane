package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// Branches inspects and removes branches of the GitHub repository the target's origin points at.
type Branches struct {
	baseURL string
	http    *http.Client
	// origin returns the origin URL of the repository at path, or "" when it has none.
	origin func(ctx context.Context, path string) (string, error)
}

func NewBranches(baseURL string, httpClient *http.Client, origin func(ctx context.Context, path string) (string, error)) *Branches {
	return &Branches{baseURL: baseURL, http: httpClient, origin: origin}
}

func (b *Branches) List(ctx context.Context, agent domain.Agent, creds domain.Credentials, prefix string) ([]string, error) {
	c, ok, err := b.client(ctx, agent, creds)
	if err != nil || !ok {
		return nil, err
	}
	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := c.do(ctx, http.MethodGet, "/git/matching-refs/heads/"+escapeRef(prefix), &refs); err != nil {
		return nil, err
	}
	var names []string
	for _, r := range refs {
		names = append(names, strings.TrimPrefix(r.Ref, "refs/heads/"))
	}
	return names, nil
}

// Inspect reads, per branch, one page of pull requests from it and one into it: a branch is the
// head or base of few.
func (b *Branches) Inspect(ctx context.Context, agent domain.Agent, creds domain.Credentials, queries []usecase.BranchQuery) ([]usecase.BranchFacts, error) {
	c, ok, err := b.client(ctx, agent, creds)
	if err != nil || !ok {
		return nil, err
	}
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.do(ctx, http.MethodGet, "", &repo); err != nil {
		return nil, err
	}
	facts := make([]usecase.BranchFacts, 0, len(queries))
	for _, q := range queries {
		f, err := c.inspect(ctx, repo.DefaultBranch, q)
		if err != nil {
			return nil, fmt.Errorf("branch %s: %w", q.Name, err)
		}
		facts = append(facts, f)
	}
	return facts, nil
}

type pull struct {
	Number   int     `json:"number"`
	Title    string  `json:"title"`
	HTMLURL  string  `json:"html_url"`
	State    string  `json:"state"`
	MergedAt *string `json:"merged_at"`
	Head     struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

func (p pull) pullRequest() usecase.PullRequest {
	return usecase.PullRequest{Number: p.Number, Title: p.Title, URL: p.HTMLURL}
}

func (c apiClient) inspect(ctx context.Context, defaultBranch string, q usecase.BranchQuery) (usecase.BranchFacts, error) {
	f := usecase.BranchFacts{Name: q.Name, Commits: make(map[string]usecase.CommitPlaces)}
	var remote struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	switch err := c.do(ctx, http.MethodGet, "/git/ref/heads/"+escapeRef(q.Name), &remote); {
	case isStatus(err, http.StatusNotFound):
	case err != nil:
		return f, err
	default:
		f.Exists = true
		f.Tip = remote.Object.SHA
	}
	owner, _, _ := strings.Cut(c.repo, "/")
	var heads, bases []pull
	if err := c.do(ctx, http.MethodGet, "/pulls?state=all&per_page=100&head="+url.QueryEscape(owner+":"+q.Name), &heads); err != nil {
		return f, err
	}
	if f.Exists {
		if err := c.do(ctx, http.MethodGet, "/pulls?state=open&per_page=100&base="+url.QueryEscape(q.Name), &bases); err != nil {
			return f, err
		}
	}
	for _, p := range bases {
		f.OpenBase = append(f.OpenBase, p.pullRequest())
	}
	var merged, closed []string
	for _, p := range heads {
		switch {
		case p.State == "open":
			f.OpenHead = append(f.OpenHead, p.pullRequest())
		case p.MergedAt != nil:
			merged = append(merged, p.Head.SHA)
		default:
			closed = append(closed, p.Head.SHA)
		}
	}
	commits := q.Commits
	if f.Exists && !slices.Contains(commits, f.Tip) {
		commits = append(slices.Clone(commits), f.Tip)
	}
	for _, commit := range commits {
		var places usecase.CommitPlaces
		var err error
		if places.InDefault, err = c.contains(ctx, escapeRef(defaultBranch), commit); err != nil {
			return f, err
		}
		if places.InMergedPull, err = c.containsAny(ctx, merged, commit); err != nil {
			return f, err
		}
		if places.InClosedPull, err = c.containsAny(ctx, closed, commit); err != nil {
			return f, err
		}
		if f.Exists {
			if places.InBranch, err = c.contains(ctx, f.Tip, commit); err != nil {
				return f, err
			}
		}
		f.Commits[commit] = places
	}
	return f, nil
}

// containsAny looks for the commit itself before comparing, as a pull request's head is usually
// the commit pushed last.
func (c apiClient) containsAny(ctx context.Context, bases []string, commit string) (bool, error) {
	if slices.Contains(bases, commit) {
		return true, nil
	}
	for _, base := range bases {
		ok, err := c.contains(ctx, base, commit)
		if ok || err != nil {
			return ok, err
		}
	}
	return false, nil
}

// contains tells whether the history of base has commit. GitHub answers 404 for a commit it does
// not have or one sharing no history with base.
func (c apiClient) contains(ctx context.Context, base, commit string) (bool, error) {
	if base == commit {
		return true, nil
	}
	aheadBy, err := c.aheadBy(ctx, base, commit)
	if isStatus(err, http.StatusNotFound) {
		return false, nil
	}
	return err == nil && aheadBy == 0, err
}

// aheadBy reads only up to ahead_by, which comes before the commits and files of a comparison that
// may run to megabytes.
func (c apiClient) aheadBy(ctx context.Context, base, head string) (int, error) {
	resp, err := c.request(ctx, http.MethodGet, "/compare/"+base+"..."+head+"?per_page=1")
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	dec := json.NewDecoder(resp.Body)
	if _, err := dec.Token(); err != nil {
		return 0, err
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return 0, err
		}
		if key == "ahead_by" {
			var n int
			return n, dec.Decode(&n)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return 0, err
		}
	}
	return 0, fmt.Errorf("github compare %s...%s: no ahead_by", base, head)
}

func (b *Branches) Remove(ctx context.Context, agent domain.Agent, creds domain.Credentials, names []string) ([]string, error) {
	c, ok, err := b.client(ctx, agent, creds)
	if err != nil || !ok {
		return nil, err
	}
	var removed []string
	for _, name := range names {
		err := c.do(ctx, http.MethodDelete, "/git/refs/heads/"+escapeRef(name), nil)
		// GitHub answers 422 for a branch already deleted, as when a merge deleted it meanwhile, and
		// for a protected one.
		if isStatus(err, http.StatusNotFound) || isStatus(err, http.StatusUnprocessableEntity) {
			continue
		}
		if err != nil {
			return removed, err
		}
		removed = append(removed, name)
	}
	return removed, nil
}

func (b *Branches) client(ctx context.Context, agent domain.Agent, creds domain.Credentials) (apiClient, bool, error) {
	if creds.GitHubToken == "" {
		return apiClient{}, false, nil
	}
	origin, err := b.origin(ctx, agent.Target.Path)
	if err != nil {
		return apiClient{}, false, err
	}
	repo, ok := parseRepo(origin)
	if !ok {
		return apiClient{}, false, nil
	}
	return apiClient{baseURL: b.baseURL, http: b.http, token: creds.GitHubToken, repo: repo}, true, nil
}

func isStatus(err error, code int) bool {
	var status statusError
	return errors.As(err, &status) && status.code == code
}

func escapeRef(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

var repoPattern = regexp.MustCompile(`^(?:https://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([^/]+/[^/]+?)(?:\.git)?/?$`)

// parseRepo returns "owner/name" of a GitHub remote URL.
func parseRepo(remote string) (string, bool) {
	m := repoPattern.FindStringSubmatch(remote)
	if m == nil {
		return "", false
	}
	return m[1], true
}

type apiClient struct {
	baseURL string
	http    *http.Client
	token   string
	repo    string
}

func (c apiClient) do(ctx context.Context, method, path string, out any) error {
	resp, err := c.request(ctx, method, path)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// request returns the response of a successful request, whose body the caller closes.
func (c apiClient) request(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/repos/"+c.repo+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, statusError{code: resp.StatusCode, msg: fmt.Sprintf("github %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(body))}
	}
	return resp, nil
}

type statusError struct {
	code int
	msg  string
}

func (e statusError) Error() string { return e.msg }
