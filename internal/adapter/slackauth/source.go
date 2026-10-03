package slackauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Source supplies the current bot token.
type Source interface {
	Token(ctx context.Context) (string, error)
}

// FileSource reads a long-lived token from a file on every call.
type FileSource string

func (f FileSource) Token(context.Context) (string, error) {
	b, err := os.ReadFile(string(f))
	if err != nil {
		return "", fmt.Errorf("slack bot token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

const DefaultBaseURL = "https://slack.com/api"

type RotatingConfig struct {
	ClientID     string
	ClientSecret string
	// InitialTokenPath holds the long-lived token exchanged for rotating ones when no state exists.
	InitialTokenPath string
	// StatePath keeps the current tokens. A refresh token works once, so it must survive restarts.
	StatePath string
	BaseURL   string
}

// RotatingSource keeps a token from Slack's token rotation fresh: access tokens live 12 hours,
// and each refresh returns a new refresh token while invalidating the one redeemed.
type RotatingSource struct {
	cfg  RotatingConfig
	http *http.Client
	now  func() time.Time

	mu    sync.Mutex
	state rotationState
}

type rotationState struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// refreshBefore is how long before expiry the token is replaced.
const refreshBefore = 2 * time.Hour

func NewRotatingSource(cfg RotatingConfig, httpClient *http.Client) *RotatingSource {
	return &RotatingSource{cfg: cfg, http: httpClient, now: time.Now}
}

func (s *RotatingSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensure(ctx); err != nil {
		return "", err
	}
	return s.state.AccessToken, nil
}

// Run refreshes the token ahead of expiry until ctx is done. A failed refresh is reported and
// retried at the next interval; the current token stays usable until it expires.
func (s *RotatingSource) Run(ctx context.Context, interval time.Duration, report func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.Token(ctx); err != nil && ctx.Err() == nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *RotatingSource) ensure(ctx context.Context) error {
	if s.state.RefreshToken == "" {
		if err := s.load(); err != nil {
			return err
		}
	}
	if s.state.RefreshToken == "" {
		initial, err := FileSource(s.cfg.InitialTokenPath).Token(ctx)
		if err != nil {
			return err
		}
		return s.redeem(ctx, "oauth.v2.exchange", url.Values{"token": {initial}})
	}
	if s.now().Add(refreshBefore).Before(s.state.ExpiresAt) {
		return nil
	}
	return s.redeem(ctx, "oauth.v2.access", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {s.state.RefreshToken},
	})
}

// RedeemCode starts over from an OAuth authorization code, for when the saved refresh token is
// lost and the long-lived token has been exchanged already.
func (s *RotatingSource) RedeemCode(ctx context.Context, code, redirectURI string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	form := url.Values{"code": {code}}
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	return s.redeem(ctx, "oauth.v2.access", form)
}

func (s *RotatingSource) load() error {
	b, err := os.ReadFile(s.cfg.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.state)
}

func (s *RotatingSource) redeem(ctx context.Context, method string, form url.Values) error {
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+"/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var r struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	if !r.OK || r.AccessToken == "" || r.RefreshToken == "" {
		return fmt.Errorf("slack %s: %s", method, r.Error)
	}
	next := rotationState{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ExpiresAt:    s.now().Add(time.Duration(r.ExpiresIn) * time.Second),
	}
	// The redeemed refresh token no longer works, so the new one is saved before it is used.
	if err := save(s.cfg.StatePath, next); err != nil {
		return fmt.Errorf("save slack tokens: %w", err)
	}
	s.state = next
	return nil
}

func save(path string, st rotationState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
