package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"go.yaml.in/yaml/v3"
)

// composeFile is the compose file whose docker-proxy service holds the allowlist (docs/adr/app/0015).
const composeFile = "../../../compose.yaml"

// proxyImage is the socket-proxy whose matching proxyAllowlist copies. When compose.yaml moves to
// another version, check that version's matching and update both.
const proxyImage = "wollomatic/socket-proxy:1.13.1"

// dockerCalls makes each dockerAPI call once. Every method of dockerAPI needs an entry, so a new
// call cannot reach production without the proxy allowlist being checked against it.
var dockerCalls = map[string]func(ctx context.Context, api dockerAPI) error{
	"ContainerCreate": func(ctx context.Context, api dockerAPI) error {
		_, err := api.ContainerCreate(ctx, client.ContainerCreateOptions{Image: "img"})
		return err
	},
	"ContainerAttach": func(ctx context.Context, api dockerAPI) error {
		res, err := api.ContainerAttach(ctx, "c1", client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
		if err == nil {
			res.Close()
		}
		return err
	},
	"ContainerStart": func(ctx context.Context, api dockerAPI) error {
		_, err := api.ContainerStart(ctx, "c1", client.ContainerStartOptions{})
		return err
	},
	"ContainerWait": func(ctx context.Context, api dockerAPI) error {
		res := api.ContainerWait(ctx, "c1", client.ContainerWaitOptions{})
		select {
		case <-res.Result:
			return nil
		case err := <-res.Error:
			return err
		}
	},
	"ContainerRemove": func(ctx context.Context, api dockerAPI) error {
		_, err := api.ContainerRemove(ctx, "c1", client.ContainerRemoveOptions{Force: true})
		return err
	},
	"ContainerList": func(ctx context.Context, api dockerAPI) error {
		_, err := api.ContainerList(ctx, client.ContainerListOptions{All: true})
		return err
	},
}

// neverAllowed are requests the runner does not send and the proxy must refuse. It samples the
// operations that give more than the runner needs (exec, writing into or reading out of containers,
// images and builds), so an entry widened with a wildcard cannot let them through while still
// counting as used. Every method the proxy allows needs at least one sample.
var neverAllowed = []engineRequest{
	{http.MethodHead, "/v1.56/containers/c1/archive"},
	{http.MethodPost, "/v1.56/containers/prune"},
	{http.MethodPost, "/v1.56/containers/c1/exec"},
	{http.MethodPost, "/v1.56/exec/e1/start"},
	{http.MethodPost, "/v1.56/containers/c1/kill"},
	{http.MethodPost, "/v1.56/containers/c1/stop"},
	{http.MethodPost, "/v1.56/containers/c1/restart"},
	{http.MethodPost, "/v1.56/containers/c1/update"},
	{http.MethodPut, "/v1.56/containers/c1/archive"},
	{http.MethodGet, "/v1.56/containers/c1/archive"},
	{http.MethodGet, "/v1.56/containers/c1/logs"},
	{http.MethodGet, "/v1.56/containers/c1/json"},
	{http.MethodPost, "/v1.56/images/create"},
	{http.MethodPost, "/v1.56/build"},
	{http.MethodDelete, "/v1.56/images/i1"},
	{http.MethodGet, "/version"},
}

// TestProxyAllowsWhatTheRunnerCalls drives the real Docker client through every dockerAPI method
// against a fake engine, and checks the requests it sends against the proxy's allowlist in
// compose.yaml: each request must be allowed, each allow entry must be used, and none may let
// through the requests in neverAllowed.
func TestProxyAllowsWhatTheRunnerCalls(t *testing.T) {
	allow := proxyAllowlist(t)

	engine := &fakeEngine{}
	srv := httptest.NewServer(engine)
	defer srv.Close()
	cli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	apiType := reflect.TypeFor[dockerAPI]()
	for i := range apiType.NumMethod() {
		name := apiType.Method(i).Name
		call, ok := dockerCalls[name]
		if !ok {
			t.Errorf("dockerAPI.%s has no entry in dockerCalls; add one so its requests are checked against the proxy", name)
			continue
		}
		if err := call(ctx, cli); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	used := make(map[*regexp.Regexp]bool)
	for _, req := range engine.requests() {
		allowed := false
		for _, re := range allow[req.method] {
			if re.MatchString(req.path) {
				allowed, used[re] = true, true
			}
		}
		if !allowed {
			t.Errorf("the proxy refuses %s %s, which the runner sends", req.method, req.path)
		}
	}
	for method, res := range allow {
		for _, re := range res {
			if !used[re] {
				t.Errorf("the proxy allows %s %s, which the runner never sends", method, entry(re))
			}
		}
	}
	sampled := make(map[string]bool)
	for _, req := range neverAllowed {
		sampled[req.method] = true
		for _, re := range allow[req.method] {
			if re.MatchString(req.path) {
				t.Errorf("the proxy allows %s %s through %s, which the runner never sends", req.method, req.path, entry(re))
			}
		}
	}
	for method := range allow {
		if !sampled[method] {
			t.Errorf("the proxy allows %s, but neverAllowed has no %s request to check its entries against", method, method)
		}
	}
}

// proxyAllowlist reads the -allow<METHOD>=<regexp> arguments of the docker-proxy service and
// compiles them as proxyImage does (compileRegexp in internal/config/config.go): wrapped
// in ^ and $, then matched against the request path without the query. Each entry must allow
// one API, without alternation, so that an unused API cannot hide in an entry that is used.
func proxyAllowlist(t *testing.T) map[string][]*regexp.Regexp {
	t.Helper()
	data, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Image   string   `yaml:"image"`
			Command []string `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	proxy, ok := compose.Services["docker-proxy"]
	if !ok {
		t.Fatalf("%s has no docker-proxy service", composeFile)
	}
	if proxy.Image != proxyImage {
		t.Fatalf("docker-proxy runs %s, but this test copies the matching of %s; check how the new version matches requests, then update proxyImage", proxy.Image, proxyImage)
	}
	allowArg := regexp.MustCompile(`^-allow([A-Z]+)=(.+)$`)
	allow := make(map[string][]*regexp.Regexp)
	for _, arg := range proxy.Command {
		m := allowArg.FindStringSubmatch(arg)
		if m == nil {
			continue
		}
		if strings.Contains(m[2], "|") {
			t.Errorf("%s: write one entry per API instead of alternatives", arg)
		}
		re, err := regexp.Compile("^" + m[2] + "$")
		if err != nil {
			t.Fatalf("%s: %v", arg, err)
		}
		allow[m[1]] = append(allow[m[1]], re)
	}
	if len(allow) == 0 {
		t.Fatalf("the docker-proxy service in %s has no -allow arguments", composeFile)
	}
	return allow
}

// entry gives back an allow entry as compose.yaml writes it, without the ^ and $ the test wraps it in.
func entry(re *regexp.Regexp) string {
	return strings.TrimSuffix(strings.TrimPrefix(re.String(), "^"), "$")
}

type engineRequest struct{ method, path string }

// fakeEngine answers the Docker Engine API calls the runner makes, and records each request.
// It fails HEAD /_ping, so the client also sends the GET /_ping it falls back to.
type fakeEngine struct {
	mu   sync.Mutex
	seen []engineRequest
}

func (e *fakeEngine) requests() []engineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]engineRequest(nil), e.seen...)
}

func (e *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	e.seen = append(e.seen, engineRequest{r.Method, r.URL.Path})
	e.mu.Unlock()

	path := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
	switch {
	case path == "/_ping" && r.Method == http.MethodHead:
		w.WriteHeader(http.StatusInternalServerError)
	case path == "/_ping":
		w.Header().Set("Api-Version", client.MaxAPIVersion)
		fmt.Fprint(w, "OK")
	case path == "/containers/create":
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"Id":"c1","Warnings":[]}`)
	case path == "/containers/json":
		fmt.Fprint(w, `[]`)
	case strings.HasSuffix(path, "/attach"):
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		fmt.Fprint(buf, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buf.Flush()
	case strings.HasSuffix(path, "/wait"):
		fmt.Fprint(w, `{"StatusCode":0}`)
	case strings.HasSuffix(path, "/start"), r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
