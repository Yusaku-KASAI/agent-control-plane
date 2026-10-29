package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const DefaultBaseURL = "https://api.github.com"

type App struct {
	ID             int64
	InstallationID int64
	PrivateKey     *rsa.PrivateKey
}

// Issuer mints an installation access token of the agent's own GitHub App on every call, and
// hands it out only when it reaches the agent's target alone.
type Issuer struct {
	apps    map[domain.AgentName]App
	baseURL string
	http    *http.Client
	now     func() time.Time
	// origin returns the origin URL of the repository at path, or "" when it has none.
	origin func(ctx context.Context, path string) (string, error)
}

func NewIssuer(apps map[domain.AgentName]App, baseURL string, httpClient *http.Client, origin func(ctx context.Context, path string) (string, error)) *Issuer {
	return &Issuer{apps: apps, baseURL: baseURL, http: httpClient, now: time.Now, origin: origin}
}

func (i *Issuer) Issue(ctx context.Context, agent domain.Agent) (domain.Credentials, error) {
	app, ok := i.apps[agent.Name]
	if !ok {
		return domain.Credentials{}, nil
	}
	jwt, err := appJWT(app, i.now())
	if err != nil {
		return domain.Credentials{}, err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", i.baseURL, app.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return domain.Credentials{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := i.http.Do(req)
	if err != nil {
		return domain.Credentials{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return domain.Credentials{}, err
	}
	if resp.StatusCode != http.StatusCreated {
		return domain.Credentials{}, fmt.Errorf("github app %d: installation token request: %s: %s",
			app.ID, resp.Status, bytes.TrimSpace(body))
	}
	var token struct {
		Token               string `json:"token"`
		RepositorySelection string `json:"repository_selection"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return domain.Credentials{}, err
	}
	if token.Token == "" {
		return domain.Credentials{}, errors.New("github returned an empty installation token")
	}
	if err := i.checkReach(ctx, app, agent, token.Token, token.RepositorySelection); err != nil {
		return domain.Credentials{}, err
	}
	return domain.Credentials{GitHubToken: token.Token}, nil
}

// checkReach refuses a token that reaches more than the agent's target: the installation decides
// the reach (ADR app/0054), so a wide one is told rather than narrowed.
func (i *Issuer) checkReach(ctx context.Context, app App, agent domain.Agent, token, selection string) error {
	origin, err := i.origin(ctx, agent.Target.Path)
	if err != nil {
		return err
	}
	target, _ := parseRepo(origin)
	repos, err := i.installationRepos(ctx, token)
	if err != nil {
		return err
	}
	if err := reachOnly(target, selection, repos); err != nil {
		return fmt.Errorf("github app %d (installation %d) of agent %q: %w; choose only the target's repository in the installation's Repository access",
			app.ID, app.InstallationID, agent.Name, err)
	}
	return nil
}

// reachOnly tells why a token whose installation has selection and repos reaches more than target
// ("owner/name", "" when the target has no GitHub origin), or nil when it reaches the target alone.
func reachOnly(target, selection string, repos []string) error {
	if target == "" {
		return errors.New("the target has no GitHub origin to hold the installation to")
	}
	if selection != "selected" {
		return fmt.Errorf("the installation reaches %q repositories, not selected ones", selection)
	}
	if len(repos) != 1 || !strings.EqualFold(repos[0], target) {
		return fmt.Errorf("the installation reaches %s, not %s alone", strings.Join(repos, ", "), target)
	}
	return nil
}

// installationRepos reads the full names of up to two repositories the token reaches: one more
// than the target is enough to tell it reaches others.
func (i *Issuer) installationRepos(ctx context.Context, token string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.baseURL+"/installation/repositories?per_page=2", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := i.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list the installation's repositories: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var list struct {
		TotalCount   int `json:"total_count"`
		Repositories []struct {
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Repositories))
	for _, r := range list.Repositories {
		names = append(names, r.FullName)
	}
	if more := list.TotalCount - len(names); more > 0 {
		names = append(names, fmt.Sprintf("%d more", more))
	}
	return names, nil
}

func appJWT(app App, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	// iat is backdated to tolerate clock drift; GitHub rejects exp more than 10 minutes ahead.
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(app.ID, 10),
	})
	if err != nil {
		return "", err
	}
	signingInput := header + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(nil, app.PrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

// LoadPrivateKey reads the PEM file GitHub issues for an App (PKCS#1, or PKCS#8 after conversion).
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an RSA key", path)
	}
	return key, nil
}
