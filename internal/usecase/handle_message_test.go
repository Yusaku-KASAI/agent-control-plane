package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type fakeTurns struct {
	out     usecase.RunAgentOutput
	err     error
	called  bool
	got     usecase.RunAgentInput
	stopped domain.SessionID
}

func (f *fakeTurns) Execute(_ context.Context, in usecase.RunAgentInput) (usecase.RunAgentOutput, error) {
	f.called, f.got = true, in
	return f.out, f.err
}

func (f *fakeTurns) Stop(_ context.Context, _ domain.AgentName, id domain.SessionID) error {
	f.stopped = id
	return nil
}

type allowList map[domain.Requester]bool

func (a allowList) Allows(_ domain.AgentName, r domain.Requester) bool { return a[r] }

type fakeResponder struct {
	acked    bool
	stopped  bool
	together bool
	replies  []string
}

func (f *fakeResponder) MarkAnsweredTogether(context.Context, usecase.IncomingMessage) error {
	f.together = true
	return nil
}

func (f *fakeResponder) MarkStopped(context.Context, usecase.IncomingMessage) error {
	f.stopped = true
	return nil
}

func (f *fakeResponder) Acknowledge(context.Context, usecase.IncomingMessage) error {
	f.acked = true
	return nil
}

func (f *fakeResponder) Reply(_ context.Context, _ usecase.IncomingMessage, text string) error {
	f.replies = append(f.replies, text)
	return nil
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
	if turns.got != want {
		t.Errorf("turn input = %+v, want %+v", turns.got, want)
	}
	if !responder.acked || len(responder.replies) != 1 || responder.replies[0] != "done" {
		t.Errorf("acked = %v, replies = %q", responder.acked, responder.replies)
	}
}

func TestHandleMessage_RejectsUnknownRequester(t *testing.T) {
	turns := &fakeTurns{}
	responder := &fakeResponder{}

	err := usecase.NewHandleMessage(turns, allowList{}, responder).Execute(context.Background(), message)
	if !errors.Is(err, domain.ErrRequesterNotAllowed) {
		t.Fatalf("Execute() error = %v, want ErrRequesterNotAllowed", err)
	}
	if turns.called || responder.acked || len(responder.replies) != 1 {
		t.Errorf("turn called = %v, acked = %v, replies = %q", turns.called, responder.acked, responder.replies)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := &fakeResponder{}
			if err := usecase.NewHandleMessage(&fakeTurns{out: tt.out, err: tt.err}, allowList{alice: true}, responder).
				Execute(context.Background(), message); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !responder.stopped || len(responder.replies) != 1 {
				t.Errorf("stopped = %v, replies = %q", responder.stopped, responder.replies)
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
	responder := &fakeResponder{}
	turns := &fakeTurns{out: usecase.RunAgentOutput{Output: "both answers", Shared: true}}
	if err := usecase.NewHandleMessage(turns, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !responder.together || len(responder.replies) != 0 {
		t.Errorf("together = %v, replies = %q; want a mark and no repeated reply", responder.together, responder.replies)
	}
}
