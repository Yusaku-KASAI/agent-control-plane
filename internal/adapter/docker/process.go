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
	"time"

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

func (p *process) Send(_ context.Context, id, prompt string) error {
	return p.write(map[string]any{
		"type":    "user",
		"uuid":    id,
		"message": map[string]any{"role": "user", "content": prompt},
	})
}

func (p *process) Interrupt(context.Context) error {
	return p.control(map[string]any{"subtype": "interrupt", "cancel_queued": true})
}

func (p *process) StopTask(_ context.Context, taskID string) error {
	return p.control(map[string]any{"subtype": "stop_task", "task_id": taskID})
}

func (p *process) control(request map[string]any) error {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return p.write(map[string]any{
		"type":       "control_request",
		"request_id": hex.EncodeToString(b),
		"request":    request,
	})
}

func (p *process) Events() <-chan usecase.ProcessEvent { return p.events }

func (p *process) Err() error { return p.err }

func (p *process) UpdateCredentials(_ context.Context, creds domain.Credentials) error {
	return p.creds.write(creds)
}

func (p *process) SetCoAuthors(_ context.Context, people []domain.GitIdentity) error {
	return p.creds.writeCoAuthors(people)
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
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	// IsError marks a result whose turn ended on an error, such as a refused API request.
	IsError bool `json:"is_error"`
	// APIError is the typed kind of the API error that ended the turn, on a result.
	APIError string `json:"api_error"`
	// RateLimitInfo is set on rate_limit_event, which reports a change in the account's usage limit.
	RateLimitInfo *struct {
		Status         string `json:"status"`
		IsUsingOverage bool   `json:"isUsingOverage"`
		// ResetsAt is in Unix seconds.
		ResetsAt int64 `json:"resetsAt"`
	} `json:"rate_limit_info"`
	Tasks []struct {
		TaskID string `json:"task_id"`
	} `json:"tasks"`
	// ParentToolUseID is set on messages from subagents.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// IsReplay marks a prompt echoed back when the agent takes it in (--replay-user-messages).
	IsReplay bool   `json:"isReplay"`
	UUID     string `json:"uuid"`
	Message  struct {
		// Content is a string in prompts and a list of blocks in assistant messages.
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// processEvent translates a stream-json line. limited tells whether the last rate_limit_event
// said requests are refused, for a result that does not name its API error.
func (e streamEvent) processEvent(limited bool) (usecase.ProcessEvent, bool) {
	switch {
	case e.Type == "system" && e.Subtype == "init":
		return usecase.ProcessEvent{Kind: usecase.EventTurnStarted}, true
	case e.Type == "system" && e.Subtype == "background_tasks_changed":
		ids := make([]string, 0, len(e.Tasks))
		for _, t := range e.Tasks {
			ids = append(ids, t.TaskID)
		}
		return usecase.ProcessEvent{Kind: usecase.EventBackgroundTasks, BackgroundTasks: ids}, true
	case e.Type == "user" && e.IsReplay:
		return usecase.ProcessEvent{Kind: usecase.EventPromptAccepted, PromptID: e.UUID}, true
	case e.Type == "assistant":
		if a := e.activity(); a != "" {
			return usecase.ProcessEvent{Kind: usecase.EventActivity, Activity: a}, true
		}
	case e.Type == "result":
		r := usecase.TurnResult{Output: e.Result}
		switch {
		case e.Subtype == "error_during_execution":
			r.Interrupted = true
		case e.IsError || strings.HasPrefix(e.Subtype, "error"):
			r.Error = true
			r.UsageLimited = e.APIError == "usage_limit_reached" || (e.APIError == "" && limited)
		}
		return usecase.ProcessEvent{Kind: usecase.EventTurnFinished, Result: r}, true
	}
	return usecase.ProcessEvent{}, false
}

// activity describes the last tool call or text in an assistant message.
func (e streamEvent) activity() string {
	var blocks []contentBlock
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return ""
	}
	var a string
	for _, c := range blocks {
		switch c.Type {
		case "tool_use":
			a = toolActivity(c.Name, c.Input)
		case "text":
			if t := strings.TrimSpace(c.Text); t != "" {
				a = firstLine(t)
			}
		}
	}
	if a != "" && e.ParentToolUseID != "" {
		a = "（サブエージェント）" + a
	}
	return a
}

func toolActivity(name string, input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
		Command     string `json:"command"`
		FilePath    string `json:"file_path"`
		Pattern     string `json:"pattern"`
		URL         string `json:"url"`
	}
	_ = json.Unmarshal(input, &in)
	for _, detail := range []string{in.Description, in.FilePath, in.Command, in.Pattern, in.URL} {
		if detail != "" {
			return name + ": " + firstLine(detail)
		}
	}
	return name
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	const max = 120
	if r := []rune(line); len(r) > max {
		return string(r[:max]) + "…"
	}
	return line
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
	var limit rateLimit
	for scanner.Scan() {
		var e streamEvent
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		if e.Type == "rate_limit_event" && e.RateLimitInfo != nil {
			limit = rateLimit{
				refused:  e.RateLimitInfo.Status == "rejected" && !e.RateLimitInfo.IsUsingOverage,
				resetsAt: e.RateLimitInfo.ResetsAt,
			}
		}
		if pe, ok := e.processEvent(limit.refusing(time.Now())); ok {
			p.events <- pe
		}
	}
	_ = stdout.CloseWithError(errors.New("stopped reading"))
	p.err = p.exitError(scanner.Err())
}

// rateLimit is what the last rate_limit_event said about the account's usage limit.
type rateLimit struct {
	refused  bool
	resetsAt int64
}

// refusing tells whether the limit still refuses requests at now; a refusal is stale once its
// reset time has passed.
func (l rateLimit) refusing(now time.Time) bool {
	return l.refused && (l.resetsAt == 0 || now.Unix() < l.resetsAt)
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
