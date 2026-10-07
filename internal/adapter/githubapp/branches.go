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

// Branches removes a session's branches from the GitHub repository the target's origin points
// at: the session branch and those named after it with a "-" suffix.
type Branches struct {
	baseURL string
	http    *http.Client
	// origin returns the origin URL of the repository at path, or "" when it has none.
	origin func(ctx context.Context, path string) (string, error)
}

func NewBranches(baseURL string, httpClient *http.Client, origin func(ctx context.Context, path string) (string, error)) *Branches {
	return &Branches{baseURL: baseURL, http: httpClient, origin: origin}
}

// OpenPullRequests and Remove do nothing without a GitHub token or a GitHub origin: the agent
// could not have pushed.
func (b *Branches) OpenPullRequests(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) ([]usecase.PullRequest, error) {
	c, branches, err := b.list(ctx, agent, id, creds)
	if err != nil || len(branches) == 0 {
		return nil, err
	}
	return c.openPullRequests(ctx, branches)
}

func (b *Branches) Remove(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) ([]string, error) {
	c, branches, err := b.list(ctx, agent, id, creds)
	if err != nil || len(branches) == 0 {
		return nil, err
	}
	pulls, err := c.openPullRequests(ctx, branches)
	if err != nil {
		return nil, err
	}
	if len(pulls) > 0 {
		return nil, usecase.ErrBranchInReview
	}
	var removed []string
	for _, name := range branches {
		err := c.do(ctx, http.MethodDelete, "/git/refs/heads/"+escapeRef(name), nil)
		// GitHub answers 422 for a branch already deleted, as when a merge deleted it meanwhile.
		var status statusError
		if errors.As(err, &status) && (status.code == http.StatusNotFound || status.code == http.StatusUnprocessableEntity) {
			continue
		}
		if err != nil {
			return removed, err
		}
		removed = append(removed, name)
	}
	return removed, nil
}

func (b *Branches) list(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) (apiClient, []string, error) {
	if creds.GitHubToken == "" {
		return apiClient{}, nil, nil
	}
	origin, err := b.origin(ctx, agent.Target.Path)
	if err != nil {
		return apiClient{}, nil, err
	}
	repo, ok := parseRepo(origin)
	if !ok {
		return apiClient{}, nil, nil
	}
	c := apiClient{baseURL: b.baseURL, http: b.http, token: creds.GitHubToken, repo: repo}
	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := c.do(ctx, http.MethodGet, "/git/matching-refs/heads/"+escapeRef(id.Branch()), &refs); err != nil {
		return apiClient{}, nil, err
	}
	var branches []string
	for _, r := range refs {
		name := strings.TrimPrefix(r.Ref, "refs/heads/")
		if name == id.Branch() || strings.HasPrefix(name, id.Branch()+"-") {
			branches = append(branches, name)
		}
	}
	return c, branches, nil
}

// openPullRequests reads one page per branch: a branch is the head of at most one open pull
// request per base, and sessions do not open many.
func (c apiClient) openPullRequests(ctx context.Context, branches []string) ([]usecase.PullRequest, error) {
	owner, _, _ := strings.Cut(c.repo, "/")
	var open []usecase.PullRequest
	for _, name := range branches {
		var pulls []struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
		}
		if err := c.do(ctx, http.MethodGet, "/pulls?state=open&per_page=100&head="+url.QueryEscape(owner+":"+name), &pulls); err != nil {
			return nil, err
		}
		for _, p := range pulls {
			open = append(open, usecase.PullRequest{Number: p.Number, Title: p.Title, URL: p.HTMLURL})
		}
	}
	return open, nil
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
