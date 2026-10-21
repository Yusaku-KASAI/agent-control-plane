package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const (
	// toolsDir is where the control plane's own binary is mounted read-only in the container.
	toolsDir = "/run/acp/bin"
	// ToolName is the file name of the control plane's binary in ToolsOptions.Dir.
	ToolName = "acp"
	// stateLabel marks the containers that read a session's workspace.
	stateLabel = "acp.workspace-state"
	// stateTimeout bounds one reading. The workspace is the agent's to write, so git may be made to
	// hang on it, by a named pipe for instance.
	stateTimeout = 2 * time.Minute
	// The agent's commands may write to the output too; what is read of it is bounded.
	stateStdoutLimit = 4 << 20
	stateStderrLimit = 16 << 10
)

// capped keeps up to limit bytes and drops the rest, telling whether it did.
type capped struct {
	buf     bytes.Buffer
	limit   int
	dropped bool
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); len(p) > room {
		c.buf.Write(p[:max(room, 0)])
		c.dropped = true
		return len(p), nil
	}
	return c.buf.Write(p)
}

// ToolsOptions is where the control plane puts its own binary for containers to run.
type ToolsOptions struct {
	Dir string
	// Volume is the Docker volume mounted at Dir; when empty, Dir is a host path and is bind-mounted.
	Volume string
}

// InstallTool copies the running binary into the tools directory, replacing the one there.
func InstallTool(opts ToolsOptions) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(opts.Dir, ToolName+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(opts.Dir, ToolName))
}

// WorkspaceReader reads what a session's workspace holds inside a throwaway container: the agent's
// image, the workspace mounted read-only where the agent sees it, no network, no credentials. The
// workspace is the agent's to write, and git runs commands its config names (core.fsmonitor,
// filters), so they must not run in the control plane.
type WorkspaceReader struct {
	api   dockerAPI
	tools ToolsOptions
}

func NewWorkspaceReader(api dockerAPI, tools ToolsOptions) *WorkspaceReader {
	return &WorkspaceReader{api: api, tools: tools}
}

func (r *WorkspaceReader) State(ctx context.Context, agent domain.Agent, workspace string) (usecase.WorkspaceState, error) {
	var w usecase.WorkspaceState
	if ok, err := pathExists(filepath.Join(workspace, ".git")); !ok || err != nil {
		return w, err
	}
	ctx, cancel := context.WithTimeout(ctx, stateTimeout)
	defer cancel()
	created, err := r.api.ContainerCreate(ctx, stateSpec(agent, workspace, r.tools))
	if err != nil {
		return w, fmt.Errorf("create container: %w", err)
	}
	defer func() {
		_, _ = r.api.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	}()
	attach, err := r.api.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdout: true, Stderr: true})
	if err != nil {
		return w, fmt.Errorf("attach container: %w", err)
	}
	defer attach.Close()
	stdout, stderr := &capped{limit: stateStdoutLimit}, &capped{limit: stateStderrLimit}
	copied := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, attach.Reader)
		copied <- err
	}()
	if _, err := r.api.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return w, fmt.Errorf("start container: %w", err)
	}
	wait := r.api.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var status int64
	select {
	case res := <-wait.Result:
		status = res.StatusCode
	case err := <-wait.Error:
		return w, fmt.Errorf("wait container: %w", err)
	case <-ctx.Done():
		return w, fmt.Errorf("read workspace: %w", ctx.Err())
	}
	select {
	case err := <-copied:
		if err != nil && !errors.Is(err, io.EOF) {
			return w, fmt.Errorf("read output: %w", err)
		}
	case <-ctx.Done():
		return w, fmt.Errorf("read output: %w", ctx.Err())
	}
	if status != 0 {
		return w, fmt.Errorf("workspace-state exited with %d: %s", status, strings.TrimSpace(stderr.buf.String()))
	}
	if stdout.dropped {
		return w, fmt.Errorf("workspace-state wrote more than %d bytes", stateStdoutLimit)
	}
	if err := json.Unmarshal(stdout.buf.Bytes(), &w); err != nil {
		return w, fmt.Errorf("decode workspace state: %w", err)
	}
	if err := checkState(w); err != nil {
		return usecase.WorkspaceState{}, fmt.Errorf("workspace state: %w", err)
	}
	return w, nil
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// checkState refuses what git would not have written. The agent can make the reading say
// anything, and the names and commits go into GitHub API paths and thread messages.
func checkState(w usecase.WorkspaceState) error {
	if w.Operation != "" && !slices.Contains(usecase.Operations, w.Operation) {
		return fmt.Errorf("unknown operation %q", w.Operation)
	}
	tips := make([]usecase.BranchTip, 0, len(w.Pushed)+len(w.Local))
	tips = append(tips, w.Pushed...)
	for _, b := range w.Local {
		tips = append(tips, b.BranchTip)
	}
	for _, b := range tips {
		if !commitPattern.MatchString(b.Tip) {
			return fmt.Errorf("branch %q: not a commit %q", b.Name, b.Tip)
		}
		if !branchName(b.Name) {
			return fmt.Errorf("not a branch name %q", b.Name)
		}
	}
	return nil
}

// branchName follows git check-ref-format for a branch, or is HEAD for a detached one.
func branchName(name string) bool {
	if name == "HEAD" {
		return true
	}
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") || strings.Contains(name, "..") ||
		strings.Contains(name, "//") || strings.Contains(name, "@{") || strings.Contains(name, "/.") || strings.HasPrefix(name, ".") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	return true
}

func stateSpec(agent domain.Agent, workspace string, tools ToolsOptions) client.ContainerCreateOptions {
	// The same place as in the agent's container, so the paths git recorded there resolve.
	workdir := path.Join(workspaceDir, filepath.Base(agent.Target.Path))
	labels := map[string]string{stateLabel: ""}
	for _, l := range composeIdentityLabels {
		labels[l] = ""
	}
	return client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        agent.Target.Image,
			Entrypoint:   []string{path.Join(toolsDir, ToolName)},
			Cmd:          []string{"workspace-state"},
			WorkingDir:   workdir,
			Labels:       labels,
			AttachStdout: true,
			AttachStderr: true,
		},
		HostConfig: &container.HostConfig{
			NetworkMode:    "none",
			CapDrop:        []string{"ALL"},
			SecurityOpt:    []string{"no-new-privileges"},
			ReadonlyRootfs: true,
			Tmpfs:          map[string]string{"/tmp": ""},
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: workspace, Target: workdir, ReadOnly: true},
				toolsMount(tools),
			},
		},
	}
}

func toolsMount(tools ToolsOptions) mount.Mount {
	if tools.Volume != "" {
		return mount.Mount{Type: mount.TypeVolume, Source: tools.Volume, Target: toolsDir, ReadOnly: true}
	}
	return mount.Mount{Type: mount.TypeBind, Source: tools.Dir, Target: toolsDir, ReadOnly: true}
}

func pathExists(p string) (bool, error) {
	_, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
