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
	var out TurnResponse
	err := c.post(ctx, turnsPath, req, &out)
	return out, err
}

func (c *Client) Stop(ctx context.Context, sessionID string, req StopRequest) error {
	return c.post(ctx, stopPath(sessionID), req, nil)
}

func (c *Client) post(ctx context.Context, path string, req, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		if json.Unmarshal(data, &apiErr.ErrorResponse) != nil || apiErr.Code == "" {
			apiErr.Code, apiErr.Message = CodeInternal, strings.TrimSpace(string(data))
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
