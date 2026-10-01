package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: httpClient}
}

// APIError is a non-2xx response from the control plane.
type APIError struct {
	Status int
	ErrorResponse
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (%d %s)", e.Message, e.Status, e.Code)
}

func (c *Client) RunTurn(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return TurnResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+turnsPath, bytes.NewReader(body))
	if err != nil {
		return TurnResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return TurnResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return TurnResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{Status: resp.StatusCode}
		if json.Unmarshal(data, &apiErr.ErrorResponse) != nil || apiErr.Code == "" {
			apiErr.Code, apiErr.Message = CodeInternal, strings.TrimSpace(string(data))
		}
		return TurnResponse{}, apiErr
	}
	var out TurnResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return TurnResponse{}, err
	}
	return out, nil
}
