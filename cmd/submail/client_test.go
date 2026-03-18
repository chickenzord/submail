package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── APIError ──────────────────────────────────────────────────────────────────

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 404, Message: "not found"}
	assert.Equal(t, "server returned 404: not found", err.Error())
}

func TestAPIError_Error_WithEmptyMessage(t *testing.T) {
	err := &APIError{StatusCode: 500, Message: ""}
	assert.Equal(t, "server returned 500: ", err.Error())
}

// ── NewClient ─────────────────────────────────────────────────────────────────

func TestNewClient_Basic(t *testing.T) {
	client := NewClient("http://example.com", "mytoken")
	require.NotNil(t, client)
	assert.Equal(t, "http://example.com", client.baseURL)
	assert.Equal(t, "mytoken", client.token)
	assert.NotNil(t, client.http)
}

func TestNewClient_StripsTrailingSlash(t *testing.T) {
	client := NewClient("http://example.com/", "token")
	assert.Equal(t, "http://example.com", client.baseURL)
}

func TestNewClient_StripsMultipleTrailingSlashes(t *testing.T) {
	client := NewClient("http://example.com///", "token")
	assert.Equal(t, "http://example.com", client.baseURL)
}

func TestNewClient_DoesNotModifyBaseURLWithoutTrailingSlash(t *testing.T) {
	client := NewClient("http://example.com", "token")
	assert.Equal(t, "http://example.com", client.baseURL)
}

// ── readAPIError ──────────────────────────────────────────────────────────────

func TestReadAPIError_JSONMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(`{"message":"something bad"}`)),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
	assert.Equal(t, "something bad", err.Message)
}

func TestReadAPIError_PlainText(t *testing.T) {
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader("bad request\n")),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
	assert.Equal(t, "bad request", err.Message)
}

func TestReadAPIError_EmptyBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	assert.Equal(t, 500, err.StatusCode)
	assert.Equal(t, "", err.Message)
}

func TestReadAPIError_IgnoresInvalidJSON(t *testing.T) {
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(`{invalid json`)),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
	assert.Equal(t, "{invalid json", err.Message)
}

func TestReadAPIError_JSONWithoutMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(`{"error":"bad","code":123}`)),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
	// Falls back to plain text since "message" field is empty
	assert.Equal(t, `{"error":"bad","code":123}`, err.Message)
}

func TestReadAPIError_LargeBody(t *testing.T) {
	// Create a body larger than 2048 bytes
	largeBody := strings.Repeat("x", 3000)
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(largeBody)),
	}
	err := readAPIError(resp)
	require.NotNil(t, err)
	// Should be limited to 2048 bytes
	assert.Equal(t, 500, err.StatusCode)
	assert.Len(t, err.Message, 2048)
}

// ── Client.ListMails ─────────────────────────────────────────────────────────

func TestClient_ListMails_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/inbox/mails", r.URL.Path)
		assert.Equal(t, "limit=10&offset=0", r.URL.RawQuery)
		assert.Equal(t, "Bearer mytoken", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		resp := ListMailsResponse{
			Total:  2,
			Limit:  10,
			Offset: 0,
			Mails: []*Mail{
				{
					ID:         "abc",
					MessageID:  "<abc@example.com>",
					Subject:    "Hello",
					From:       "sender@example.com",
					To:         "recipient@example.com",
					ReceivedAt: time.Date(2024, 3, 18, 10, 0, 0, 0, time.UTC),
					TextBody:   "Hello world",
					HTMLBody:   "<p>Hello world</p>",
				},
				{
					ID:        "def",
					MessageID: "<def@example.com>",
					Subject:   "Re: Hello",
					From:      "recipient@example.com",
					To:        "sender@example.com",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "mytoken")
	resp, err := client.ListMails(context.Background(), 10, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 10, resp.Limit)
	assert.Equal(t, 0, resp.Offset)
	assert.Len(t, resp.Mails, 2)
	assert.Equal(t, "abc", resp.Mails[0].ID)
	assert.Equal(t, "Hello", resp.Mails[0].Subject)
}

func TestClient_ListMails_EmptyList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := ListMailsResponse{
			Total:  0,
			Limit:  10,
			Offset: 0,
			Mails:  []*Mail{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	resp, err := client.ListMails(context.Background(), 10, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 0, resp.Total)
	assert.Len(t, resp.Mails, 0)
}

func TestClient_ListMails_Error401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "unauthorized"})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "badtoken")
	resp, err := client.ListMails(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.IsType(t, &APIError{}, err)
	apiErr := err.(*APIError)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.Equal(t, "unauthorized", apiErr.Message)
}

func TestClient_ListMails_Error500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "internal server error"})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	resp, err := client.ListMails(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, resp)
	apiErr := err.(*APIError)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestClient_ListMails_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	resp, err := client.ListMails(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "decode response")
}

func TestClient_ListMails_ContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListMailsResponse{})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := client.ListMails(ctx, 10, 0)
	require.Error(t, err)
	assert.Nil(t, resp)
}

// ── Client.GetMail ────────────────────────────────────────────────────────────

func TestClient_GetMail_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/inbox/mails/abc123", r.URL.Path)
		assert.Equal(t, "Bearer mytoken", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		mail := Mail{
			ID:         "abc123",
			MessageID:  "<abc123@example.com>",
			Subject:    "Test Email",
			From:       "sender@example.com",
			To:         "recipient@example.com",
			ReceivedAt: time.Date(2024, 3, 18, 10, 0, 0, 0, time.UTC),
			TextBody:   "This is the body",
			HTMLBody:   "<p>This is the body</p>",
		}
		json.NewEncoder(w).Encode(mail)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "mytoken")
	mail, err := client.GetMail(context.Background(), "abc123")
	require.NoError(t, err)
	require.NotNil(t, mail)
	assert.Equal(t, "abc123", mail.ID)
	assert.Equal(t, "Test Email", mail.Subject)
	assert.Equal(t, "sender@example.com", mail.From)
	assert.Equal(t, "This is the body", mail.TextBody)
}

func TestClient_GetMail_Error404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	mail, err := client.GetMail(context.Background(), "missing")
	require.Error(t, err)
	assert.Nil(t, mail)
	assert.IsType(t, &APIError{}, err)
	apiErr := err.(*APIError)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "not found", apiErr.Message)
}

func TestClient_GetMail_Error403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "forbidden"})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "badtoken")
	mail, err := client.GetMail(context.Background(), "someone_elses_mail")
	require.Error(t, err)
	assert.Nil(t, mail)
	apiErr := err.(*APIError)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestClient_GetMail_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	mail, err := client.GetMail(context.Background(), "123")
	require.Error(t, err)
	assert.Nil(t, mail)
	assert.Contains(t, err.Error(), "decode response")
}

func TestClient_GetMail_URLEscape(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The http library decodes the path before it reaches the handler
		// so "abc 123" (after url.PathEscape) becomes "abc 123" in r.URL.Path
		assert.Equal(t, "/api/v1/inbox/mails/abc 123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Mail{ID: "abc 123"})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	mail, err := client.GetMail(context.Background(), "abc 123")
	require.NoError(t, err)
	assert.Equal(t, "abc 123", mail.ID)
}

func TestClient_GetMail_ContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Mail{})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mail, err := client.GetMail(ctx, "123")
	require.Error(t, err)
	assert.Nil(t, mail)
}
