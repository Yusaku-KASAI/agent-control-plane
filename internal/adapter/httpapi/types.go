package httpapi

const turnsPath = "/v1/turns"

func stopPath(sessionID string) string { return "/v1/sessions/" + sessionID + "/stop" }

type TurnRequest struct {
	Agent     string `json:"agent"`
	SessionID string `json:"session_id,omitempty"`
	Prompt    string `json:"prompt"`
	// Inject sends the prompt into the running turn instead of queueing it.
	Inject bool `json:"inject,omitempty"`
}

type TurnResponse struct {
	SessionID     string `json:"session_id"`
	WorkspacePath string `json:"workspace_path"`
	Output        string `json:"output"`
	Interrupted   bool   `json:"interrupted,omitempty"`
}

type StopRequest struct {
	Agent string `json:"agent"`
}

// ErrorResponse carries the session, when one exists, so a failed turn can still be inspected.
type ErrorResponse struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	SessionID     string `json:"session_id,omitempty"`
	WorkspacePath string `json:"workspace_path,omitempty"`
}

const (
	CodeInvalidRequest       = "invalid_request"
	CodeUnauthorized         = "unauthorized"
	CodeAgentNotFound        = "agent_not_found"
	CodeSessionNotFound      = "session_not_found"
	CodeSessionAgentMismatch = "session_agent_mismatch"
	CodeSessionBusy          = "session_busy"
	CodeInboxFull            = "inbox_full"
	CodeUnavailable          = "unavailable"
	CodeInternal             = "internal"
)
