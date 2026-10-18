package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type fakeTurns struct {
	out     usecase.RunAgentOutput
	err     error
	taken   usecase.Uptake
	called  bool
	got     usecase.RunAgentInput
	stopped domain.SessionID
}

func (f *fakeTurns) Execute(_ context.Context, in usecase.RunAgentInput) (usecase.RunAgentOutput, error) {
	f.called = true
	if f.taken != 0 {
		in.Taken(f.taken)
	}
	// The callback is checked through the responder; drop it so the input can be compared.
	in.Taken = nil
	f.got = in
	return f.out, f.err
}

func (f *fakeTurns) Stop(_ context.Context, _ domain.AgentName, id domain.SessionID) error {
	f.stopped = id
	return nil
}

type allowList map[domain.Requester]bool

func (a allowList) Allows(_ domain.AgentName, r domain.Requester) bool { return a[r] }

// fakeResponder records the marks on the message and the replies, in order, as their names; a
// reply is recorded as "reply" in marks and by its text in replies.
type fakeResponder struct {
	marks   []string
	replies []string
}

func (f *fakeResponder) mark(name string) error {
	f.marks = append(f.marks, name)
	return nil
}

func (f *fakeResponder) Acknowledge(context.Context, usecase.IncomingMessage) error {
	return f.mark("received")
}

func (f *fakeResponder) MarkStarted(context.Context, usecase.IncomingMessage) error {
	return f.mark("started")
}

func (f *fakeResponder) MarkInjected(context.Context, usecase.IncomingMessage) error {
	return f.mark("injected")
}

func (f *fakeResponder) MarkAnswered(context.Context, usecase.IncomingMessage) error {
	return f.mark("answered")
}

func (f *fakeResponder) MarkFailed(context.Context, usecase.IncomingMessage) error {
	return f.mark("failed")
}

func (f *fakeResponder) MarkStopped(context.Context, usecase.IncomingMessage) error {
	return f.mark("stopped")
}

func (f *fakeResponder) Reply(_ context.Context, _ usecase.IncomingMessage, text string) error {
	f.replies = append(f.replies, text)
	return f.mark("reply")
}

var (
	alice   = domain.Requester{Platform: "slack", ID: "U_ALICE"}
	thread  = domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.000"}
	message = usecase.IncomingMessage{Agent: "repo-dev", Conversation: thread, Requester: alice, MessageID: "1.000", Text: "fix it"}
)

func TestHandleMessage_RunsTurnInThreadSession(t *testing.T) {
	turns := &fakeTurns{out: usecase.RunAgentOutput{Output: "done"}}
	responder := &fakeResponder{}

	if err := usecase.NewHandleMessage(turns, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := usecase.RunAgentInput{
		AgentName: "repo-dev", SessionID: thread.SessionID("repo-dev"), CreateIfMissing: true, Prompt: "fix it", Conversation: thread,
		Requester: alice,
	}
	if !reflect.DeepEqual(turns.got, want) {
		t.Errorf("turn input = %+v, want %+v", turns.got, want)
	}
	if !slices.Equal(responder.marks, []string{"received", "reply", "answered"}) || !slices.Equal(responder.replies, []string{"done"}) {
		t.Errorf("marks = %q, replies = %q", responder.marks, responder.replies)
	}
}

func TestHandleMessage_MarksHowTheRequestWasTaken(t *testing.T) {
	tests := []struct {
		name  string
		taken usecase.Uptake
		want  []string
	}{
		{name: "never told", want: []string{"received", "reply", "answered"}},
		{name: "began a turn", taken: usecase.TakenIntoNewTurn, want: []string{"received", "started", "reply", "answered"}},
		{name: "taken into the running turn", taken: usecase.TakenIntoRunningTurn, want: []string{"received", "injected", "reply", "answered"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turns := &fakeTurns{out: usecase.RunAgentOutput{Output: "done"}, taken: tt.taken}
			responder := &fakeResponder{}
			if err := usecase.NewHandleMessage(turns, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !slices.Equal(responder.marks, tt.want) {
				t.Errorf("marks = %q, want %q", responder.marks, tt.want)
			}
		})
	}
}

func TestHandleMessage_RejectsUnknownRequester(t *testing.T) {
	turns := &fakeTurns{}
	responder := &fakeResponder{}

	err := usecase.NewHandleMessage(turns, allowList{}, responder).Execute(context.Background(), message)
	if !errors.Is(err, domain.ErrRequesterNotAllowed) {
		t.Fatalf("Execute() error = %v, want ErrRequesterNotAllowed", err)
	}
	if turns.called || !slices.Equal(responder.marks, []string{"reply"}) || len(responder.replies) != 1 {
		t.Errorf("turn called = %v, marks = %q, replies = %q", turns.called, responder.marks, responder.replies)
	}
}

func TestHandleMessage_RepliesOnFailure(t *testing.T) {
	tests := []struct {
		name string
		out  usecase.RunAgentOutput
		err  error
		want string
	}{
		{name: "inbox full", err: usecase.ErrInboxFull, want: "処理待ちの依頼が多すぎます"},
		{
			name: "failed turn",
			out:  usecase.RunAgentOutput{Session: domain.Session{ID: "0f8fad5b-d9cb-469f-a165-70867728950e"}},
			err:  errors.New("agent exited with status 1"),
			want: "セッション 0f8fad5b-d9cb-469f-a165-70867728950e",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := &fakeResponder{}
			err := usecase.NewHandleMessage(&fakeTurns{out: tt.out, err: tt.err}, allowList{alice: true}, responder).
				Execute(context.Background(), message)
			if !errors.Is(err, tt.err) {
				t.Errorf("Execute() error = %v, want %v", err, tt.err)
			}
			if len(responder.replies) != 1 || !strings.Contains(responder.replies[0], tt.want) {
				t.Errorf("replies = %q, want one containing %q", responder.replies, tt.want)
			}
			if !slices.Equal(responder.marks, []string{"received", "reply", "failed"}) {
				t.Errorf("marks = %q, want received, the reply and then failed", responder.marks)
			}
		})
	}
}

func TestHandleMessage_Stopped(t *testing.T) {
	tests := []struct {
		name string
		out  usecase.RunAgentOutput
		err  error
	}{
		{name: "running turn interrupted", out: usecase.RunAgentOutput{Interrupted: true}},
		{name: "queued request withdrawn", err: usecase.ErrStopped},
		{name: "cut short by a restart", err: usecase.ErrShuttingDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := &fakeResponder{}
			if err := usecase.NewHandleMessage(&fakeTurns{out: tt.out, err: tt.err}, allowList{alice: true}, responder).
				Execute(context.Background(), message); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !slices.Equal(responder.marks, []string{"received", "reply", "stopped"}) || len(responder.replies) != 1 {
				t.Errorf("marks = %q, replies = %q", responder.marks, responder.replies)
			}
		})
	}
}

func TestHandleMessage_Stop(t *testing.T) {
	turns := &fakeTurns{}
	h := usecase.NewHandleMessage(turns, allowList{alice: true}, &fakeResponder{})
	id := thread.SessionID("repo-dev")

	if err := h.Stop(context.Background(), "repo-dev", domain.Requester{Platform: "slack", ID: "U_BOB"}, id); !errors.Is(err, domain.ErrRequesterNotAllowed) || turns.stopped != "" {
		t.Errorf("Stop(unknown requester) error = %v, stopped %q", err, turns.stopped)
	}
	if err := h.Stop(context.Background(), "repo-dev", alice, id); err != nil || turns.stopped != id {
		t.Errorf("Stop() error = %v, stopped %q", err, turns.stopped)
	}
}

func TestHandleMessage_AnsweredTogether(t *testing.T) {
	tests := []struct {
		name string
		out  usecase.RunAgentOutput
		want []string
	}{
		{name: "answered", out: usecase.RunAgentOutput{Output: "both answers", Shared: true}, want: []string{"received", "answered"}},
		{name: "ended on an error", out: usecase.RunAgentOutput{Output: "API Error: 500", Shared: true, Error: true}, want: []string{"received", "failed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := &fakeResponder{}
			if err := usecase.NewHandleMessage(&fakeTurns{out: tt.out}, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !slices.Equal(responder.marks, tt.want) || len(responder.replies) != 0 {
				t.Errorf("marks = %q, replies = %q; want %q and no repeated reply", responder.marks, responder.replies, tt.want)
			}
		})
	}
}

func TestHandleMessage_TurnEndedOnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  usecase.RunAgentOutput
		want []string
	}{
		{"usage limit", usecase.RunAgentOutput{Output: "You've hit your session limit · resets 2pm (Asia/Tokyo)", Error: true, UsageLimited: true},
			[]string{"使用上限", "もう一度依頼", "> You've hit your session limit · resets 2pm (Asia/Tokyo)"}},
		{"other error", usecase.RunAgentOutput{Output: "API Error: 500\nretry later", Error: true},
			[]string{"エラーでターンが終わりました", "> API Error: 500\n> retry later"}},
	} {
		responder := &fakeResponder{}
		turns := &fakeTurns{out: tc.out}
		if err := usecase.NewHandleMessage(turns, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
			t.Fatalf("%s: Execute() error = %v", tc.name, err)
		}
		if len(responder.replies) != 1 {
			t.Fatalf("%s: replies = %q, want one", tc.name, responder.replies)
		}
		if !slices.Equal(responder.marks, []string{"received", "reply", "failed"}) {
			t.Errorf("%s: marks = %q, want received, the reply and then failed", tc.name, responder.marks)
		}
		for _, w := range tc.want {
			if !strings.Contains(responder.replies[0], w) {
				t.Errorf("%s: reply lacks %q:\n%s", tc.name, w, responder.replies[0])
			}
		}
	}
}
