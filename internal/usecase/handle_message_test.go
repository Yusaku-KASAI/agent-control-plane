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
	out            usecase.RunAgentOutput
	err            error
	taken          usecase.Uptake
	called         bool
	got            usecase.RunAgentInput
	stopped        domain.SessionID
	stoppedRequest string
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

func (f *fakeTurns) StopRequest(_ context.Context, _ domain.AgentName, id domain.SessionID, requestID string) error {
	f.stopped, f.stoppedRequest = id, requestID
	return nil
}

type allowList map[domain.Requester]bool

func (a allowList) Allows(_ domain.AgentName, r domain.Requester) bool { return a[r] }

// fakeResponder records the marks on the message and the replies, in order, as their names; a
// reply is recorded as "reply" in marks and by its text in replies.
type fakeResponder struct {
	marks   []string
	replies []string
	// refuse makes a reply with this text fail, after recording it.
	refuse string
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
	if text == f.refuse {
		f.marks = append(f.marks, "reply refused")
		return errors.New("refused")
	}
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

	if err := usecase.NewHandleMessage(turns, nil, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := usecase.RunAgentInput{
		AgentName: "repo-dev", SessionID: thread.SessionID("repo-dev"), CreateIfMissing: true, Prompt: "fix it", Conversation: thread,
		Requester: alice, RequestID: "1.000",
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
			if err := usecase.NewHandleMessage(turns, nil, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
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

	err := usecase.NewHandleMessage(turns, nil, allowList{}, responder).Execute(context.Background(), message)
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
			err := usecase.NewHandleMessage(&fakeTurns{out: tt.out, err: tt.err}, nil, allowList{alice: true}, responder).
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
			if err := usecase.NewHandleMessage(&fakeTurns{out: tt.out, err: tt.err}, nil, allowList{alice: true}, responder).
				Execute(context.Background(), message); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !slices.Equal(responder.marks, []string{"received", "reply", "stopped"}) || len(responder.replies) != 1 {
				t.Errorf("marks = %q, replies = %q", responder.marks, responder.replies)
			}
		})
	}
}

func TestHandleMessage_StopRequest(t *testing.T) {
	turns := &fakeTurns{}
	h := usecase.NewHandleMessage(turns, nil, allowList{alice: true}, &fakeResponder{})

	if err := h.StopRequest(context.Background(), "repo-dev", domain.Requester{Platform: "slack", ID: "U_BOB"}, thread, "2.000"); !errors.Is(err, domain.ErrRequesterNotAllowed) || turns.stopped != "" {
		t.Errorf("StopRequest(unknown requester) error = %v, stopped %q", err, turns.stopped)
	}
	if err := h.StopRequest(context.Background(), "repo-dev", alice, thread, "2.000"); err != nil || turns.stopped != thread.SessionID("repo-dev") || turns.stoppedRequest != "2.000" {
		t.Errorf("StopRequest() error = %v, stopped %q request %q", err, turns.stopped, turns.stoppedRequest)
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
			if err := usecase.NewHandleMessage(&fakeTurns{out: tt.out}, nil, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
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
		if err := usecase.NewHandleMessage(turns, nil, allowList{alice: true}, responder).Execute(context.Background(), message); err != nil {
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

type cleanCalls struct {
	ids    []domain.SessionID
	forced []string
}

func (c *cleanCalls) CleanNow(_ context.Context, id domain.SessionID, _ domain.Requester) error {
	c.ids = append(c.ids, id)
	return nil
}

func (c *cleanCalls) ForceClean(_ context.Context, _ domain.AgentName, _ domain.Requester, _ domain.Conversation, message string) error {
	c.forced = append(c.forced, message)
	return nil
}

// Only a person who may use the agent can have its session cleaned.
func TestHandleMessage_CleanSession(t *testing.T) {
	cleanup := &cleanCalls{}
	h := usecase.NewHandleMessage(&fakeTurns{}, cleanup, allowList{alice: true}, &fakeResponder{})
	c := domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"}

	bob := domain.Requester{Platform: "slack", ID: "U_BOB"}
	if err := h.CleanSession(context.Background(), "dev", bob, c); !errors.Is(err, domain.ErrRequesterNotAllowed) {
		t.Errorf("CleanSession() by someone else = %v, want ErrRequesterNotAllowed", err)
	}
	if err := h.ForceCleanSession(context.Background(), "dev", bob, c, "2.0"); !errors.Is(err, domain.ErrRequesterNotAllowed) {
		t.Errorf("ForceCleanSession() by someone else = %v, want ErrRequesterNotAllowed", err)
	}
	if err := h.CleanSession(context.Background(), "dev", alice, c); err != nil || len(cleanup.ids) != 1 || cleanup.ids[0] != c.SessionID("dev") {
		t.Errorf("CleanSession() = %v, cleaned %v; want the thread's session", err, cleanup.ids)
	}
	if err := h.ForceCleanSession(context.Background(), "dev", alice, c, "2.0"); err != nil || !slices.Equal(cleanup.forced, []string{"2.0"}) {
		t.Errorf("ForceCleanSession() = %v, forced %v; want the notice", err, cleanup.forced)
	}
}

// An answer that cannot be posted is told by a short notice, and the message is marked failed instead
// of answered, so that it does not look answered (#348).
func TestHandleMessage_TellsAnAnswerNotPosted(t *testing.T) {
	session := domain.Session{ID: "s-1"}
	turns := &fakeTurns{out: usecase.RunAgentOutput{Output: "done", Session: session}}
	responder := &fakeResponder{refuse: "done"}

	if err := usecase.NewHandleMessage(turns, nil, allowList{alice: true}, responder).Execute(context.Background(), message); err == nil {
		t.Error("Execute() = nil, want the reply's error for the log")
	}
	if !slices.Equal(responder.marks, []string{"received", "reply refused", "reply", "failed"}) {
		t.Errorf("marks = %q, want the notice and the failed mark after the refused reply", responder.marks)
	}
	if len(responder.replies) != 2 || !strings.Contains(responder.replies[1], "投稿できませんでした") || !strings.Contains(responder.replies[1], "s-1") {
		t.Errorf("replies = %q, want the notice naming the session after the reply", responder.replies)
	}
}

// A notice that is no answer, such as that the request was stopped, has no answer to write again: one
// that cannot be posted keeps its mark and adds no notice.
func TestHandleMessage_KeepsTheMarkOfANoticeNotPosted(t *testing.T) {
	turns := &fakeTurns{err: usecase.ErrStopped}
	responder := &fakeResponder{refuse: "停止したため、この依頼は取り消しました。"}

	_ = usecase.NewHandleMessage(turns, nil, allowList{alice: true}, responder).Execute(context.Background(), message)
	if !slices.Equal(responder.marks, []string{"received", "reply refused", "stopped"}) {
		t.Errorf("marks = %q, want the stopped mark after the refused notice", responder.marks)
	}
}
