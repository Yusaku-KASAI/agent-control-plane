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
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

const DefaultBaseURL = "https://api.github.com"

type App struct {
	ID             int64
	InstallationID int64
	PrivateKey     *rsa.PrivateKey
}

// Issuer mints an installation access token of the agent's own GitHub App on every call.
type Issuer struct {
	apps    map[domain.AgentName]App
	baseURL string
	http    *http.Client
	now     func() time.Time
}

func NewIssuer(apps map[domain.AgentName]App, baseURL string, httpClient *http.Client) *Issuer {
	return &Issuer{apps: apps, baseURL: baseURL, http: httpClient, now: time.Now}
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
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return domain.Credentials{}, err
	}
	if token.Token == "" {
		return domain.Credentials{}, errors.New("github returned an empty installation token")
	}
	return domain.Credentials{GitHubToken: token.Token}, nil
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
