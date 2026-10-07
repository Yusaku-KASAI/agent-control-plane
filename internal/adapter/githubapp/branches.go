package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
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

// Remove does nothing without a GitHub token or a GitHub origin: the agent could not have pushed.
func (b *Branches) Remove(ctx context.Context, agent domain.Agent, id domain.SessionID, creds domain.Credentials) error {
	if creds.GitHubToken == "" {
		return nil
	}
	origin, err := b.origin(ctx, agent.Target.Path)
	if err != nil {
		return err
	}
	repo, ok := parseRepo(origin)
	if !ok {
		return nil
	}
	c := apiClient{baseURL: b.baseURL, http: b.http, token: creds.GitHubToken, repo: repo}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := c.do(ctx, http.MethodGet, "/git/matching-refs/heads/"+id.Branch(), &refs); err != nil {
		return err
	}
	var branches []string
	for _, r := range refs {
		name := strings.TrimPrefix(r.Ref, "refs/heads/")
		if name == id.Branch() || strings.HasPrefix(name, id.Branch()+"-") {
			branches = append(branches, name)
		}
	}
	for _, name := range branches {
		var pulls []json.RawMessage
		head := url.QueryEscape(strings.SplitN(repo, "/", 2)[0] + ":" + name)
		if err := c.do(ctx, http.MethodGet, "/pulls?state=open&per_page=1&head="+head, &pulls); err != nil {
			return err
		}
		if len(pulls) > 0 {
			return fmt.Errorf("%w: %s", usecase.ErrBranchInReview, name)
		}
	}
	for _, name := range branches {
		if err := c.do(ctx, http.MethodDelete, "/git/refs/heads/"+name, nil); err != nil {
			return err
		}
	}
	return nil
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
		return fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
