package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// readerAPI is a container that writes stdout and stderr, then exits with status, or never exits
// when status is nil.
type readerAPI struct {
	stdout, stderr string
	status         *int64

	mu      sync.Mutex
	created client.ContainerCreateOptions
	removed []string
}

func (f *readerAPI) ContainerCreate(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = opts
	return client.ContainerCreateResult{ID: "r1"}, nil
}

func (f *readerAPI) ContainerAttach(context.Context, string, client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
	local, _ := net.Pipe()
	r, w := io.Pipe()
	go func() {
		for _, out := range []struct {
			stream byte
			data   string
		}{{1, f.stdout}, {2, f.stderr}} {
			if out.data == "" {
				continue
			}
			header := make([]byte, 8)
			header[0] = out.stream
			binary.BigEndian.PutUint32(header[4:], uint32(len(out.data)))
			_, _ = w.Write(append(header, out.data...))
		}
		if f.status != nil {
			_ = w.Close()
		}
	}()
	return client.ContainerAttachResult{HijackedResponse: client.HijackedResponse{Conn: local, Reader: bufio.NewReader(r)}}, nil
}

func (f *readerAPI) ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, nil
}

func (f *readerAPI) ContainerWait(context.Context, string, client.ContainerWaitOptions) client.ContainerWaitResult {
	res := make(chan container.WaitResponse, 1)
	if f.status != nil {
		res <- container.WaitResponse{StatusCode: *f.status}
	}
	return client.ContainerWaitResult{Result: res, Error: make(chan error)}
}

func (f *readerAPI) ContainerRemove(_ context.Context, id string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return client.ContainerRemoveResult{}, nil
}

func (f *readerAPI) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}

func repoWorkspace(t *testing.T) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(ws, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return ws
}

const tip = "0123456789abcdef0123456789abcdef01234567"

var readerAgent = domain.Agent{Name: "repo-dev", Target: domain.Target{Path: "/src/repo", Image: "repo-dev:latest"}}

func TestWorkspaceReader_State(t *testing.T) {
	ok := int64(0)
	api := &readerAPI{stdout: `{"Changes":true,"Operation":"merge","Local":[{"Name":"HEAD","Tip":"` + tip + `","Unpushed":true}]}` + "\n", status: &ok}
	ws := repoWorkspace(t)
	tools := ToolsOptions{Dir: "/var/lib/acp/tools", Volume: "acp-tools"}

	got, err := NewWorkspaceReader(api, tools).State(context.Background(), readerAgent, ws)

	want := usecase.WorkspaceState{Changes: true, Operation: "merge",
		Local: []usecase.LocalBranch{{BranchTip: usecase.BranchTip{Name: "HEAD", Tip: tip}, Unpushed: true}}}
	if err != nil || got.Changes != want.Changes || got.Operation != want.Operation || !slices.Equal(got.Local, want.Local) {
		t.Errorf("State() = %+v, %v; want %+v", got, err, want)
	}
	if !slices.Equal(api.removed, []string{"r1"}) {
		t.Errorf("removed %v, want the container", api.removed)
	}

	cfg, host := api.created.Config, api.created.HostConfig
	if agentCfg := containerSpec(usecase.StartRequest{Agent: readerAgent, Session: domain.Session{ID: "0f8fad5b-d9cb-469f-a165-70867728950e"}}, nil, Options{}).Config; cfg.WorkingDir != agentCfg.WorkingDir {
		t.Errorf("working directory = %q, want the agent's %q", cfg.WorkingDir, agentCfg.WorkingDir)
	}
	if cfg.Image != "repo-dev:latest" || !slices.Equal(cfg.Entrypoint, []string{"/run/acp/bin/acp"}) ||
		!slices.Equal(cfg.Cmd, []string{"workspace-state"}) || cfg.WorkingDir != "/workspace/repo" || len(cfg.Env) != 0 {
		t.Errorf("config = %+v, want the agent's image running the tool in the agent's working directory, without env", cfg)
	}
	if _, ok := cfg.Labels[stateLabel]; !ok {
		t.Errorf("labels = %v, want %s", cfg.Labels, stateLabel)
	}
	if host.NetworkMode != "none" || !slices.Equal(host.CapDrop, []string{"ALL"}) || !host.ReadonlyRootfs ||
		!slices.Contains(host.SecurityOpt, "no-new-privileges") || host.Memory == 0 || host.PidsLimit == nil ||
		*host.PidsLimit <= 0 || host.Tmpfs["/tmp"] == "" {
		t.Errorf("host config = %+v, want no network, no capabilities, a read-only root, bounded resources", host)
	}
	wantMounts := []mount.Mount{
		{Type: mount.TypeBind, Source: ws, Target: "/workspace/repo", ReadOnly: true},
		{Type: mount.TypeVolume, Source: "acp-tools", Target: "/run/acp/bin", ReadOnly: true},
	}
	if !slices.Equal(host.Mounts, wantMounts) {
		t.Errorf("mounts = %+v, want only the workspace and the tools, read-only", host.Mounts)
	}
}

func TestWorkspaceReader_State_Failed(t *testing.T) {
	failed := int64(1)
	api := &readerAPI{stderr: "acp: git status: fatal\n", status: &failed}

	_, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, repoWorkspace(t))

	if err == nil || !strings.Contains(err.Error(), "git status: fatal") {
		t.Errorf("State() error = %v, want the tool's stderr", err)
	}
	if mounts := api.created.HostConfig.Mounts; mounts[1] != (mount.Mount{Type: mount.TypeBind, Source: "/tools", Target: "/run/acp/bin", ReadOnly: true}) {
		t.Errorf("tools mount = %+v, want the host directory bind-mounted", mounts[1])
	}
	if !slices.Equal(api.removed, []string{"r1"}) {
		t.Errorf("removed %v, want the container", api.removed)
	}
}

// The agent may make git hang, by a named pipe for instance; the reading gives up and removes it.
func TestWorkspaceReader_State_Hangs(t *testing.T) {
	api := &readerAPI{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(ctx, readerAgent, repoWorkspace(t))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("State() error = %v, want the deadline", err)
	}
	if !slices.Equal(api.removed, []string{"r1"}) {
		t.Errorf("removed %v, want the container", api.removed)
	}
}

func TestWorkspaceReader_State_NoRepository(t *testing.T) {
	api := &readerAPI{}

	got, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, t.TempDir())

	if err != nil || got.Changes || api.created.Config != nil {
		t.Errorf("State() = %+v, %v, created %+v; want nothing read and no container", got, err, api.created)
	}
}

func TestInstallTool(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ToolName), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := InstallTool(ToolsOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(self)
	got, err := os.ReadFile(filepath.Join(dir, ToolName))
	info, statErr := os.Stat(filepath.Join(dir, ToolName))
	if err != nil || statErr != nil || string(got) != string(want) || info.Mode().Perm() != 0o755 {
		t.Errorf("installed %d bytes, mode %v, %v; want the running binary, executable", len(got), info, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left %d files, want only the tool", len(entries))
	}
}

// The agent's commands may write to the output; too much of it is an error, not a large read.
func TestWorkspaceReader_State_TooMuchOutput(t *testing.T) {
	ok := int64(0)
	api := &readerAPI{stdout: strings.Repeat("x", stateStdoutLimit+1), stderr: strings.Repeat("e", stateStderrLimit*2), status: &ok}

	_, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, repoWorkspace(t))

	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("State() error = %v, want the output refused", err)
	}
}

func TestWorkspaceReader_State_FailedStderrBounded(t *testing.T) {
	failed := int64(1)
	api := &readerAPI{stderr: strings.Repeat("e", stateStderrLimit*4), status: &failed}

	_, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, repoWorkspace(t))

	if err == nil || len(err.Error()) > stateStderrLimit+100 {
		t.Errorf("State() error of %d bytes, want it bounded by the stderr limit", len(fmt.Sprint(err)))
	}
}

// What the agent can make the reading say is refused unless git could have written it.
func TestWorkspaceReader_State_Refused(t *testing.T) {
	ok := int64(0)
	for name, out := range map[string]string{
		"commit in a path":  `{"Local":[{"Name":"x","Tip":"../../../../user","Unpushed":true}]}`,
		"name with ..":      `{"Pushed":[{"Name":"a/../b","Tip":"` + tip + `"}]}`,
		"name with newline": `{"Local":[{"Name":"a\nb","Tip":"` + tip + `"}]}`,
		"name with a space": `{"Pushed":[{"Name":"a b","Tip":"` + tip + `"}]}`,
		"unknown operation": `{"Operation":"rm -rf"}`,
		"name ending .lock": `{"Pushed":[{"Name":"a.lock","Tip":"` + tip + `"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			api := &readerAPI{stdout: out, status: &ok}
			if _, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, repoWorkspace(t)); err == nil {
				t.Errorf("State() of %s = nil error, want it refused", out)
			}
		})
	}
	api := &readerAPI{stdout: `{"Pushed":[{"Name":"acp/0f8fad5b-fix","Tip":"` + tip + `"}],"Operation":"cherry-pick"}`, status: &ok}
	if _, err := NewWorkspaceReader(api, ToolsOptions{Dir: "/tools"}).State(context.Background(), readerAgent, repoWorkspace(t)); err != nil {
		t.Errorf("State() of a session branch = %v, want it read", err)
	}
}
