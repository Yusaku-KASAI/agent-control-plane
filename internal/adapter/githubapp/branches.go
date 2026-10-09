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

// Inspect reads, per branch, one page of its pull requests: a branch is the head or base of few.
func (b *Branches) Inspect(ctx context.Context, agent domain.Agent, creds domain.Credentials, refs []usecase.BranchRef) ([]usecase.RemoteBranch, error) {
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
	var branches []usecase.RemoteBranch
	for _, ref := range refs {
		if ref.Name == repo.DefaultBranch {
			continue
		}
		br, err := c.inspect(ctx, repo.DefaultBranch, ref)
		if err != nil {
			return nil, fmt.Errorf("branch %s: %w", ref.Name, err)
		}
		branches = append(branches, br)
	}
	return branches, nil
}

type pull struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Head    struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

func (c apiClient) inspect(ctx context.Context, defaultBranch string, ref usecase.BranchRef) (usecase.RemoteBranch, error) {
	br := usecase.RemoteBranch{BranchRef: ref, State: usecase.BranchUnfinished}
	var remote struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	tip := ref.LocalTip
	switch err := c.do(ctx, http.MethodGet, "/git/ref/heads/"+escapeRef(ref.Name), &remote); {
	case isStatus(err, http.StatusNotFound):
	case err != nil:
		return br, err
	default:
		br.Exists = true
		tip = remote.Object.SHA
	}
	owner, _, _ := strings.Cut(c.repo, "/")
	var heads, bases []pull
	if err := c.do(ctx, http.MethodGet, "/pulls?state=all&per_page=100&head="+url.QueryEscape(owner+":"+ref.Name), &heads); err != nil {
		return br, err
	}
	if br.Exists && ref.Owned {
		if err := c.do(ctx, http.MethodGet, "/pulls?state=open&per_page=100&base="+url.QueryEscape(ref.Name), &bases); err != nil {
			return br, err
		}
	}
	var closed []string
	for _, p := range append(heads, bases...) {
		if p.State == "open" {
			br.Pulls = append(br.Pulls, usecase.PullRequest{Number: p.Number, Title: p.Title, URL: p.HTMLURL})
		} else {
			closed = append(closed, p.Head.SHA)
		}
	}
	if len(br.Pulls) > 0 {
		br.State = usecase.BranchInReview
		return br, nil
	}
	if !ref.Owned && br.Exists {
		br.State = usecase.BranchDone
		return br, nil
	}
	for _, head := range closed {
		contained, err := c.contains(ctx, head, tip)
		if err != nil {
			return br, err
		}
		if contained {
			br.State = usecase.BranchDone
			return br, nil
		}
	}
	contained, err := c.contains(ctx, escapeRef(defaultBranch), tip)
	if err != nil {
		return br, err
	}
	if contained {
		br.State = usecase.BranchDone
	}
	return br, nil
}

// contains tells whether the history of base has commit, as when a pull request got more commits
// after the session last pulled. GitHub answers 404 for a commit it does not have or one sharing no
// history with base.
func (c apiClient) contains(ctx context.Context, base, commit string) (bool, error) {
	if base == commit {
		return true, nil
	}
	var cmp struct {
		AheadBy int `json:"ahead_by"`
	}
	err := c.do(ctx, http.MethodGet, "/compare/"+base+"..."+commit, &cmp)
	if isStatus(err, http.StatusNotFound) {
		return false, nil
	}
	return err == nil && cmp.AheadBy == 0, err
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

// client is not ok without a GitHub token or a GitHub origin: the agent could not have pushed.
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
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/repos/"+c.repo+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return statusError{code: resp.StatusCode, msg: fmt.Sprintf("github %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(body))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

type statusError struct {
	code int
	msg  string
}

func (e statusError) Error() string { return e.msg }
