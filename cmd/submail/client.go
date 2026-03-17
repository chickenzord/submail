package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Mail is the client-side representation of an email returned by the API.
type Mail struct {
	ID         string    `json:"id"`
	MessageID  string    `json:"message_id"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ReceivedAt time.Time `json:"received_at"`
	TextBody   string    `json:"text_body,omitempty"`
	HTMLBody   string    `json:"html_body,omitempty"`
}

// ListMailsResponse is the envelope returned by GET /api/v1/inbox/mails.
type ListMailsResponse struct {
	Mails  []*Mail `json:"mails"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// APIError represents a non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
}

// Client is a thin HTTP client for the Submail API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a new API client for the given server URL and Bearer token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req)
}

// ListMails fetches a paginated list of mails from the inbox.
func (c *Client) ListMails(ctx context.Context, limit, offset int) (*ListMailsResponse, error) {
	resp, err := c.do(ctx, fmt.Sprintf("/api/v1/inbox/mails?limit=%d&offset=%d", limit, offset))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, readAPIError(resp)
	}
	var out ListMailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

// GetMail fetches a single mail by its storage ID.
func (c *Client) GetMail(ctx context.Context, id string) (*Mail, error) {
	resp, err := c.do(ctx, "/api/v1/inbox/mails/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, readAPIError(resp)
	}
	var out Mail
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

func readAPIError(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	var v struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &v) == nil && v.Message != "" {
		return &APIError{StatusCode: resp.StatusCode, Message: v.Message}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
}
