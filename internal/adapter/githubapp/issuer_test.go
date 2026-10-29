package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

var dev = domain.Agent{Name: "repo-dev", Role: "developer", Target: domain.Target{Path: "/targets/repo"}}

func originOf(url string) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return url, nil }
}

// installationAPI answers the token request with selection, and the repository list with repos.
func installationAPI(t *testing.T, selection string, repos ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/99/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "ghs_abc", "repository_selection": selection})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer ghs_abc" {
				http.Error(w, "wrong token", http.StatusUnauthorized)
				return
			}
			page := repos
			if n, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && n < len(page) {
				page = page[:n]
			}
			list := []map[string]string{}
			for _, name := range page {
				list = append(list, map[string]string{"full_name": name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": len(repos), "repositories": list})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAppJWT(t *testing.T) {
	key := newKey(t)
	now := time.Unix(1_700_000_000, 0)

	jwt, err := appJWT(App{ID: 42, PrivateKey: key}, now)
	if err != nil {
		t.Fatalf("appJWT() error = %v", err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Errorf("signature does not verify: %v", err)
	}

	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != "42" || claims.Iat >= now.Unix() || claims.Exp-now.Unix() > 600 {
		t.Errorf("claims = %+v", claims)
	}
}

func TestIssuer_Issue(t *testing.T) {
	key := newKey(t)
	var gotPath, gotAuth string
	api := installationAPI(t, "selected", "Owner/Repo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		}
		api.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	issuer := NewIssuer(map[domain.AgentName]App{dev.Name: {ID: 1, InstallationID: 99, PrivateKey: key}}, srv.URL, srv.Client(),
		originOf("git@github.com:owner/repo.git"))
	got, err := issuer.Issue(context.Background(), dev)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if got.GitHubToken != "ghs_abc" {
		t.Errorf("GitHubToken = %q", got.GitHubToken)
	}
	if gotPath != "/app/installations/99/access_tokens" || !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("request = %s (auth %q)", gotPath, gotAuth)
	}
}

func TestIssuer_Issue_RefusesAWideInstallation(t *testing.T) {
	for name, tt := range map[string]struct {
		origin, selection string
		repos             []string
		want, fix         string
	}{
		"all repositories":             {"https://github.com/owner/repo.git", "all", []string{"owner/repo"}, `"all"`, "Repository access"},
		"another repository selected":  {"https://github.com/owner/repo.git", "selected", []string{"owner/repo", "owner/other"}, "owner/other", "Repository access"},
		"only another one":             {"https://github.com/owner/repo.git", "selected", []string{"owner/other"}, "not owner/repo alone", "Repository access"},
		"more than a page":             {"https://github.com/owner/repo.git", "selected", []string{"owner/repo", "owner/a", "owner/b"}, "1 more", "Repository access"},
		"no selection in the response": {"https://github.com/owner/repo.git", "", []string{"owner/repo"}, `""`, "Repository access"},
		"a target without GitHub":      {"/srv/git/repo.git", "selected", []string{"owner/repo"}, "no GitHub origin", "origin at its GitHub repository"},
		"a target without an origin":   {"", "selected", []string{"owner/repo"}, "no GitHub origin", "origin at its GitHub repository"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := installationAPI(t, tt.selection, tt.repos...)
			issuer := NewIssuer(map[domain.AgentName]App{dev.Name: {ID: 1, InstallationID: 99, PrivateKey: newKey(t)}}, srv.URL, srv.Client(),
				originOf(tt.origin))
			got, err := issuer.Issue(context.Background(), dev)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), tt.fix) {
				t.Errorf("Issue() error = %v, want one naming %s and telling to fix %s", err, tt.want, tt.fix)
			}
			if got != (domain.Credentials{}) {
				t.Errorf("Issue() = %+v, want no credentials", got)
			}
		})
	}
}

func TestIssuer_Issue_Failures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	issuer := NewIssuer(map[domain.AgentName]App{dev.Name: {ID: 1, InstallationID: 99, PrivateKey: newKey(t)}}, srv.URL, srv.Client(),
		originOf("https://github.com/owner/repo.git"))

	if _, err := issuer.Issue(context.Background(), dev); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("Issue() error = %v, want the GitHub message", err)
	}
	if got, err := issuer.Issue(context.Background(), domain.Agent{Name: "no-app"}); err != nil || got != (domain.Credentials{}) {
		t.Errorf("Issue(agent without app) = %+v, %v; want empty credentials", got, err)
	}
}

func TestLoadPrivateKey(t *testing.T) {
	key := newKey(t)
	dir := t.TempDir()
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*pem.Block{
		"pkcs1.pem": {Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)},
		"pkcs8.pem": {Type: "PRIVATE KEY", Bytes: pkcs8},
	}
	for name, block := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadPrivateKey(path)
		if err != nil || !got.Equal(key) {
			t.Errorf("LoadPrivateKey(%s) = %v, %v", name, got != nil, err)
		}
	}
}
