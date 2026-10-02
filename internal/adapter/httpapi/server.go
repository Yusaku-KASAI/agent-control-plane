package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

type TurnRunner interface {
	Execute(ctx context.Context, in usecase.RunAgentInput) (usecase.RunAgentOutput, error)
	Interrupt(ctx context.Context, agent domain.AgentName, id domain.SessionID) error
}

func NewHandler(turns TurnRunner, token string, log *slog.Logger) http.Handler {
	s := &server{turns: turns, token: token, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("POST "+turnsPath, s.authenticated(http.HandlerFunc(s.postTurn)))
	mux.Handle("POST "+interruptPath("{id}"), s.authenticated(http.HandlerFunc(s.postInterrupt)))
	return mux
}

type server struct {
	turns TurnRunner
	token string
	log   *slog.Logger
}

func (s *server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Code: CodeUnauthorized, Message: "missing or invalid API token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) postTurn(w http.ResponseWriter, r *http.Request) {
	var req TurnRequest
	if !decode(w, r, &req) {
		return
	}
	in := usecase.RunAgentInput{AgentName: domain.AgentName(req.Agent), Prompt: req.Prompt, Inject: req.Inject}
	if req.SessionID != "" {
		id, err := domain.ParseSessionID(req.SessionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Code: CodeInvalidRequest, Message: err.Error()})
			return
		}
		in.SessionID = id
	}

	out, err := s.turns.Execute(r.Context(), in)
	if err != nil {
		status, code := classify(err)
		if status == http.StatusInternalServerError {
			s.log.Error("turn failed", "agent", req.Agent, "session", out.Session.ID, "err", err)
		}
		writeJSON(w, status, ErrorResponse{
			Code:          code,
			Message:       err.Error(),
			SessionID:     string(out.Session.ID),
			WorkspacePath: out.Session.Workspace.Path,
		})
		return
	}
	writeJSON(w, http.StatusOK, TurnResponse{
		SessionID:     string(out.Session.ID),
		WorkspacePath: out.Session.Workspace.Path,
		Output:        out.Output,
		Interrupted:   out.Interrupted,
	})
}

func (s *server) postInterrupt(w http.ResponseWriter, r *http.Request) {
	id, err := domain.ParseSessionID(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Code: CodeInvalidRequest, Message: err.Error()})
		return
	}
	var req InterruptRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.turns.Interrupt(r.Context(), domain.AgentName(req.Agent), id); err != nil {
		status, code := classify(err)
		writeJSON(w, status, ErrorResponse{Code: code, Message: err.Error(), SessionID: string(id)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Code: CodeInvalidRequest, Message: err.Error()})
		return false
	}
	return true
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, usecase.ErrEmptyPrompt):
		return http.StatusBadRequest, CodeInvalidRequest
	case errors.Is(err, domain.ErrAgentNotFound):
		return http.StatusNotFound, CodeAgentNotFound
	case errors.Is(err, domain.ErrSessionNotFound):
		return http.StatusNotFound, CodeSessionNotFound
	case errors.Is(err, domain.ErrSessionAgentMismatch):
		return http.StatusConflict, CodeSessionAgentMismatch
	case errors.Is(err, domain.ErrSessionBusy):
		return http.StatusConflict, CodeSessionBusy
	case errors.Is(err, usecase.ErrInboxFull):
		return http.StatusTooManyRequests, CodeInboxFull
	case errors.Is(err, usecase.ErrShuttingDown):
		return http.StatusServiceUnavailable, CodeUnavailable
	default:
		return http.StatusInternalServerError, CodeInternal
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
