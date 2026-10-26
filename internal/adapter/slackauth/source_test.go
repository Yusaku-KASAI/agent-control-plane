package slackauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeSlack issues numbered token pairs and accepts each refresh token once.
type fakeSlack struct {
	mu       sync.Mutex
	issued   int
	valid    map[string]bool
	calls    []string
	exchange string
}

func (f *fakeSlack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = r.ParseForm()
	f.calls = append(f.calls, r.URL.Path)
	if r.Form.Get("client_secret") != "secret" {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_client"}`))
		return
	}
	switch r.URL.Path {
	case "/oauth.v2.exchange":
		f.exchange = r.Form.Get("token")
	case "/oauth.v2.access":
		if r.Form.Get("code") != "" {
			if r.Form.Get("code") != "good-code" {
				_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
				return
			}
			break
		}
		if !f.valid[r.Form.Get("refresh_token")] {
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_refresh_token"}`))
			return
		}
		delete(f.valid, r.Form.Get("refresh_token"))
	}
	f.issued++
	refresh := "refresh-" + string(rune('0'+f.issued))
	f.valid[refresh] = true
	_, _ = w.Write([]byte(`{"ok":true,"access_token":"xoxe.xoxb-` + string(rune('0'+f.issued)) + `","refresh_token":"` + refresh + `","expires_in":43200}`))
}

func newSource(t *testing.T, srv *httptest.Server, dir string) *RotatingSource {
	t.Helper()
	initial := filepath.Join(dir, "bot-token")
	if err := os.WriteFile(initial, []byte("xoxb-long-lived\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewRotatingSource(RotatingConfig{
		ClientID: "id", ClientSecret: "secret", InitialTokenPath: initial,
		StatePath: filepath.Join(dir, "state", "agent.json"), BaseURL: srv.URL,
	}, srv.Client())
}

func TestRotatingSource(t *testing.T) {
	slack := &fakeSlack{valid: map[string]bool{}}
	srv := httptest.NewServer(slack)
	defer srv.Close()
	dir := t.TempDir()
	ctx := context.Background()

	s := newSource(t, srv, dir)
	start := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return start }
	if tok, err := s.Token(ctx); err != nil || tok != "xoxe.xoxb-1" {
		t.Fatalf("first Token() = %q, %v; want the exchanged token", tok, err)
	}
	if slack.exchange != "xoxb-long-lived" {
		t.Errorf("exchanged %q, want the long-lived token", slack.exchange)
	}
	if tok, _ := s.Token(ctx); tok != "xoxe.xoxb-1" || len(slack.calls) != 1 {
		t.Errorf("Token() before expiry = %q after %d calls, want no refresh", tok, len(slack.calls))
	}
	if err := s.refresh(ctx); err != nil || len(slack.calls) != 1 {
		t.Errorf("refresh() long before expiry = %v after %d calls, want no refresh", err, len(slack.calls))
	}

	// Within two hours of expiry, Token still returns the current token; Run's refresh replaces it
	// with the saved refresh token.
	s.now = func() time.Time { return start.Add(11 * time.Hour) }
	if tok, _ := s.Token(ctx); tok != "xoxe.xoxb-1" || len(slack.calls) != 1 {
		t.Errorf("Token() near expiry = %q after %d calls, want the current token without a refresh", tok, len(slack.calls))
	}
	if err := s.refresh(ctx); err != nil {
		t.Fatalf("refresh() near expiry error = %v", err)
	}
	if tok, err := s.Token(ctx); err != nil || tok != "xoxe.xoxb-2" {
		t.Fatalf("Token() after refresh = %q, %v; want the refreshed token", tok, err)
	}

	// A restarted control plane continues from the saved state, not from the initial token; Token
	// refreshes a token that has expired.
	restarted := newSource(t, srv, dir)
	restarted.now = func() time.Time { return start.Add(23 * time.Hour) }
	if tok, err := restarted.Token(ctx); err != nil || tok != "xoxe.xoxb-3" {
		t.Fatalf("Token() after restart = %q, %v; want a refresh from the saved state", tok, err)
	}
	if got := slack.calls; len(got) != 3 || got[2] != "/oauth.v2.access" {
		t.Errorf("calls = %q", got)
	}
}

func TestRotatingSource_RefreshFails(t *testing.T) {
	slack := &fakeSlack{valid: map[string]bool{}}
	srv := httptest.NewServer(slack)
	defer srv.Close()
	ctx := context.Background()

	s := newSource(t, srv, t.TempDir())
	start := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return start }
	if _, err := s.Token(ctx); err != nil {
		t.Fatal(err)
	}
	clear(slack.valid)

	// A failed refresh ahead of expiry is Run's to report; posting goes on with the current token.
	s.now = func() time.Time { return start.Add(11 * time.Hour) }
	// Run returns on the report, or after the timeout when it reports nothing.
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var reported error
	s.Run(runCtx, time.Hour, func(err error) {
		reported = err
		cancel()
	})
	if reported == nil {
		t.Error("Run() reported no failed refresh")
	}
	if tok, err := s.Token(ctx); err != nil || tok != "xoxe.xoxb-1" {
		t.Errorf("Token() after a failed refresh = %q, %v; want the current token", tok, err)
	}

	// Once the token has expired, there is none to return; Slack counts its expiry from earlier
	// than the recorded one.
	s.now = func() time.Time { return start.Add(12*time.Hour - 30*time.Second) }
	if tok, err := s.Token(ctx); err == nil {
		t.Errorf("Token() after expiry with a failed refresh = %q, want an error", tok)
	}
}

func TestRotatingSource_Errors(t *testing.T) {
	srv := httptest.NewServer(&fakeSlack{valid: map[string]bool{}})
	defer srv.Close()
	s := newSource(t, srv, t.TempDir())
	s.cfg.ClientSecret = "wrong"
	if _, err := s.Token(context.Background()); err == nil {
		t.Fatal("Token() with a wrong secret error = nil")
	}
	if _, err := os.Stat(s.cfg.StatePath); !os.IsNotExist(err) {
		t.Errorf("state was saved after a failed exchange (stat err = %v)", err)
	}
}

func TestRotatingSource_RedeemCode(t *testing.T) {
	slack := &fakeSlack{valid: map[string]bool{}}
	srv := httptest.NewServer(slack)
	defer srv.Close()
	dir := t.TempDir()
	ctx := context.Background()

	s := newSource(t, srv, dir)
	if err := s.RedeemCode(ctx, "bad-code", ""); err == nil {
		t.Fatal("RedeemCode(bad) error = nil")
	}
	if err := s.RedeemCode(ctx, "good-code", "https://localhost/cb"); err != nil {
		t.Fatalf("RedeemCode() error = %v", err)
	}

	// The control plane, restarted, continues from the redeemed tokens without exchanging.
	restarted := newSource(t, srv, dir)
	if tok, err := restarted.Token(ctx); err != nil || tok != "xoxe.xoxb-1" {
		t.Errorf("Token() after recovery = %q, %v", tok, err)
	}
	if slack.exchange != "" {
		t.Errorf("exchanged %q after recovery, want no exchange", slack.exchange)
	}
}
