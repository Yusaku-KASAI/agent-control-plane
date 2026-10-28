package httpapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

const (
	token     = "secret"
	sessionID = "0f8fad5b-d9cb-469f-a165-70867728950e"
)

type fakeTurns struct {
	out usecase.RunAgentOutput
	err error
	got usecase.RunAgentInput

	stopped domain.SessionID
	stopErr error
}

func (f *fakeTurns) Execute(_ context.Context, in usecase.RunAgentInput) (usecase.RunAgentOutput, error) {
	f.got = in
	return f.out, f.err
}

func (f *fakeTurns) Stop(_ context.Context, _ domain.AgentName, id domain.SessionID) error {
	f.stopped = id
	return f.stopErr
}

func newServer(t *testing.T, turns *fakeTurns) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httpapi.NewHandler(turns, token, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return srv
}

var session = domain.Session{ID: sessionID, AgentName: "repo-dev", Workspace: domain.Workspace{Path: "/s/ws/repo"}}

func TestRunTurn(t *testing.T) {
	turns := &fakeTurns{out: usecase.RunAgentOutput{Output: "done", Session: session}}
	srv := newServer(t, turns)

	got, err := httpapi.NewClient(srv.URL, token, srv.Client()).RunTurn(context.Background(),
		httpapi.TurnRequest{Agent: "repo-dev", SessionID: sessionID, Prompt: "fix it", Inject: true})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	want := httpapi.TurnResponse{SessionID: sessionID, WorkspacePath: "/s/ws/repo", Output: "done"}
	if got != want {
		t.Errorf("RunTurn() = %+v, want %+v", got, want)
	}
	wantIn := usecase.RunAgentInput{AgentName: "repo-dev", SessionID: sessionID, Prompt: "fix it", Inject: true}
	if !reflect.DeepEqual(turns.got, wantIn) {
		t.Errorf("usecase got %+v, want %+v", turns.got, wantIn)
	}
}

func TestRunTurn_Errors(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		req        httpapi.TurnRequest
		err        error
		wantStatus int
		wantCode   string
		wantSess   string
	}{
		{name: "wrong token", token: "nope", wantStatus: 401, wantCode: httpapi.CodeUnauthorized},
		{name: "invalid session id", token: token, req: httpapi.TurnRequest{SessionID: "../x"}, wantStatus: 400, wantCode: httpapi.CodeInvalidRequest},
		{name: "empty prompt", token: token, err: usecase.ErrEmptyPrompt, wantStatus: 400, wantCode: httpapi.CodeInvalidRequest},
		{name: "unknown agent", token: token, err: domain.ErrAgentNotFound, wantStatus: 404, wantCode: httpapi.CodeAgentNotFound},
		{name: "unknown session", token: token, err: domain.ErrSessionNotFound, wantStatus: 404, wantCode: httpapi.CodeSessionNotFound},
		{name: "other agent's session", token: token, err: domain.ErrSessionAgentMismatch, wantStatus: 409, wantCode: httpapi.CodeSessionAgentMismatch},
		{name: "busy session", token: token, err: domain.ErrSessionBusy, wantStatus: 409, wantCode: httpapi.CodeSessionBusy},
		{name: "inbox full", token: token, err: usecase.ErrInboxFull, wantStatus: 429, wantCode: httpapi.CodeInboxFull},
		{name: "stopped", token: token, err: usecase.ErrStopped, wantStatus: 500, wantCode: httpapi.CodeInternal, wantSess: sessionID},
		{name: "shutting down", token: token, err: usecase.ErrShuttingDown, wantStatus: 503, wantCode: httpapi.CodeUnavailable, wantSess: sessionID},
		{
			name: "failed turn keeps the session", token: token, err: fmt.Errorf("run agent: %w", errors.New("exit 1")),
			wantStatus: 500, wantCode: httpapi.CodeInternal, wantSess: sessionID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, &fakeTurns{out: usecase.RunAgentOutput{Session: session}, err: tt.err})
			_, err := httpapi.NewClient(srv.URL, tt.token, srv.Client()).RunTurn(context.Background(), tt.req)

			var apiErr *httpapi.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("RunTurn() error = %v, want *APIError", err)
			}
			if apiErr.Status != tt.wantStatus || apiErr.Code != tt.wantCode {
				t.Errorf("error = %d %s, want %d %s", apiErr.Status, apiErr.Code, tt.wantStatus, tt.wantCode)
			}
			if tt.wantSess != "" && apiErr.SessionID != tt.wantSess {
				t.Errorf("SessionID = %q, want %q", apiErr.SessionID, tt.wantSess)
			}
		})
	}
}

func TestStop(t *testing.T) {
	turns := &fakeTurns{}
	srv := newServer(t, turns)
	c := httpapi.NewClient(srv.URL, token, srv.Client())

	if err := c.Stop(context.Background(), sessionID, httpapi.StopRequest{Agent: "repo-dev"}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if turns.stopped != sessionID {
		t.Errorf("stopped %q, want %q", turns.stopped, sessionID)
	}

	turns.stopErr = domain.ErrSessionAgentMismatch
	var apiErr *httpapi.APIError
	if err := c.Stop(context.Background(), sessionID, httpapi.StopRequest{Agent: "repo-pm"}); !errors.As(err, &apiErr) || apiErr.Code != httpapi.CodeSessionAgentMismatch {
		t.Errorf("Stop(other agent) error = %v", err)
	}
	if err := c.Stop(context.Background(), "bad", httpapi.StopRequest{Agent: "repo-dev"}); !errors.As(err, &apiErr) || apiErr.Code != httpapi.CodeInvalidRequest {
		t.Errorf("Stop(bad id) error = %v", err)
	}
}

func TestHealthz(t *testing.T) {
	srv := newServer(t, &fakeTurns{})
	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
