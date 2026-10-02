package docker

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// process speaks Claude Code's stream-json protocol over the container's attached stdin/stdout.
type process struct {
	api    dockerAPI
	id     string
	attach client.HijackedResponse
	creds  sessionCredentials

	writeMu sync.Mutex
	events  chan usecase.ProcessEvent
	done    chan struct{}
	err     error
	stderr  tailBuffer
}

func newProcess(api dockerAPI, id string, attach client.HijackedResponse, creds sessionCredentials) *process {
	p := &process{
		api: api, id: id, attach: attach, creds: creds,
		events: make(chan usecase.ProcessEvent, 16),
		done:   make(chan struct{}),
	}
	go p.read()
	return p
}

func (p *process) Send(_ context.Context, prompt string) error {
	return p.write(map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": prompt},
	})
}

func (p *process) Interrupt(context.Context) error {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return p.write(map[string]any{
		"type":       "control_request",
		"request_id": hex.EncodeToString(b),
		"request":    map[string]any{"subtype": "interrupt"},
	})
}

func (p *process) Events() <-chan usecase.ProcessEvent { return p.events }

func (p *process) Err() error { return p.err }

func (p *process) UpdateCredentials(_ context.Context, creds domain.Credentials) error {
	return p.creds.write(creds)
}

func (p *process) Close(ctx context.Context) error {
	_ = p.attach.CloseWrite()
	select {
	case <-p.done:
	case <-ctx.Done():
	}
	_, err := p.api.ContainerRemove(context.WithoutCancel(ctx), p.id, client.ContainerRemoveOptions{Force: true})
	p.attach.Close()
	return err
}

func (p *process) write(msg any) error {
	line, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.attach.Conn.Write(append(line, '\n'))
	return err
}

// streamEvent is the part of a stream-json output line the runner needs.
type streamEvent struct {
	Type    string            `json:"type"`
	Subtype string            `json:"subtype"`
	Result  string            `json:"result"`
	Tasks   []json.RawMessage `json:"tasks"`
}

func (e streamEvent) processEvent() (usecase.ProcessEvent, bool) {
	switch {
	case e.Type == "system" && e.Subtype == "init":
		return usecase.ProcessEvent{Kind: usecase.EventTurnStarted}, true
	case e.Type == "system" && e.Subtype == "background_tasks_changed":
		return usecase.ProcessEvent{Kind: usecase.EventBackgroundTasks, BackgroundTasks: len(e.Tasks)}, true
	case e.Type == "result":
		return usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: usecase.TurnResult{
			Output:      e.Result,
			Interrupted: e.Subtype == "error_during_execution",
		}}, true
	}
	return usecase.ProcessEvent{}, false
}

func (p *process) read() {
	defer close(p.done)
	defer close(p.events)

	stdout, w := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(w, &p.stderr, p.attach.Reader)
		w.CloseWithError(err)
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var e streamEvent
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		if pe, ok := e.processEvent(); ok {
			p.events <- pe
		}
	}
	_ = stdout.CloseWithError(errors.New("stopped reading"))
	p.err = p.exitError(scanner.Err())
}

func (p *process) exitError(readErr error) error {
	wait := p.api.ContainerWait(context.Background(), p.id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case res := <-wait.Result:
		if res.StatusCode != 0 {
			return fmt.Errorf("agent exited with status %d: %s", res.StatusCode, strings.TrimSpace(p.stderr.String()))
		}
		return readErr
	case err := <-wait.Error:
		return errors.Join(readErr, fmt.Errorf("wait for container: %w", err))
	}
}

// tailBuffer keeps the last bytes written, enough to explain a failure.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

const tailSize = 8 * 1024

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > tailSize {
		t.buf = t.buf[len(t.buf)-tailSize:]
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
