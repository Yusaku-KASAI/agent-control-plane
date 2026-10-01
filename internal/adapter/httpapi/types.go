package httpapi

const turnsPath = "/v1/turns"

type TurnRequest struct {
	Agent     string `json:"agent"`
	SessionID string `json:"session_id,omitempty"`
	Prompt    string `json:"prompt"`
}

type TurnResponse struct {
	SessionID     string `json:"session_id"`
	WorkspacePath string `json:"workspace_path"`
	Output        string `json:"output"`
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
	CodeInternal             = "internal"
)
