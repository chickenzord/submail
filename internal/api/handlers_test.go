package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEnv holds a server and its backing store for handler tests.
type testEnv struct {
	srv   *api.Server
	store storage.Store
	cfg   *config.Config
}

var testAgents = []config.AgentConfig{
	{ID: "alpha", Token: "token-alpha", Addresses: []string{"bot+alpha@example.com"}},
	{ID: "beta", Token: "token-beta", Addresses: []string{"bot+beta@example.com"}},
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{Agents: testAgents}
	return &testEnv{
		srv:   api.NewServer(cfg, store),
		store: store,
		cfg:   cfg,
	}
}

func (e *testEnv) do(method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) seedMessage(t *testing.T, msg *storage.Message) *storage.Message {
	t.Helper()
	require.NoError(t, e.store.SaveMessage(t.Context(), msg))
	return msg
}

// sampleMessage returns a ready-to-save message addressed to the given recipient.
func sampleMessage(messageID, subject, to string, age time.Duration) *storage.Message {
	return &storage.Message{
		MessageID:  messageID,
		Subject:    subject,
		From:       "sender@example.com",
		To:         to,
		ReceivedAt: time.Now().Add(-age),
		TextBody:   "body of " + subject,
	}
}

// --- Auth ---

func TestListMails_MissingToken(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListMails_InvalidToken(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "not-a-real-token")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetMail_MissingToken(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails/someid", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- List mails ---

func TestListMails_Empty(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-alpha")

	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Mails)
	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 50, resp.Limit)
	assert.Equal(t, 0, resp.Offset)
}

func TestListMails_OnlyReturnsAgentMails(t *testing.T) {
	env := newTestEnv(t)

	env.seedMessage(t, sampleMessage("<a1@test>", "For alpha", "bot+alpha@example.com", time.Hour))
	env.seedMessage(t, sampleMessage("<a2@test>", "For alpha 2", "bot+alpha@example.com", 2*time.Hour))
	env.seedMessage(t, sampleMessage("<b1@test>", "For beta", "bot+beta@example.com", time.Hour))

	// alpha sees only its own 2 messages
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Mails, 2)
	for _, m := range resp.Mails {
		assert.Equal(t, "bot+alpha@example.com", m.To)
	}

	// beta sees only its own 1 message
	rec = env.do(http.MethodGet, "/api/v1/inbox/mails", "token-beta")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Total)
	assert.Len(t, resp.Mails, 1)
	assert.Equal(t, "bot+beta@example.com", resp.Mails[0].To)
}

func TestListMails_OrderedByReceivedAtDesc(t *testing.T) {
	env := newTestEnv(t)

	env.seedMessage(t, sampleMessage("<old@test>", "Oldest", "bot+alpha@example.com", 3*time.Hour))
	env.seedMessage(t, sampleMessage("<mid@test>", "Middle", "bot+alpha@example.com", 2*time.Hour))
	env.seedMessage(t, sampleMessage("<new@test>", "Newest", "bot+alpha@example.com", 1*time.Hour))

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Mails, 3)
	assert.Equal(t, "Newest", resp.Mails[0].Subject)
	assert.Equal(t, "Middle", resp.Mails[1].Subject)
	assert.Equal(t, "Oldest", resp.Mails[2].Subject)
}

func TestListMails_Pagination(t *testing.T) {
	env := newTestEnv(t)

	for i := range 5 {
		env.seedMessage(t, sampleMessage(
			"<pag"+string(rune('A'+i))+"@test>",
			"Mail "+string(rune('A'+i)),
			"bot+alpha@example.com",
			time.Duration(5-i)*time.Hour,
		))
	}

	// First page
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails?limit=2&offset=0", "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)
	var page1 api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))
	assert.Equal(t, 5, page1.Total)
	assert.Len(t, page1.Mails, 2)
	assert.Equal(t, 2, page1.Limit)
	assert.Equal(t, 0, page1.Offset)

	// Second page
	rec = env.do(http.MethodGet, "/api/v1/inbox/mails?limit=2&offset=2", "token-alpha")
	var page2 api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page2))
	assert.Equal(t, 5, page2.Total)
	assert.Len(t, page2.Mails, 2)
	assert.Equal(t, 2, page2.Offset)

	// Pages don't overlap
	page1IDs := []string{page1.Mails[0].ID, page1.Mails[1].ID}
	for _, m := range page2.Mails {
		assert.NotContains(t, page1IDs, m.ID)
	}
}

// --- Get mail ---

func TestGetMail_NotFound(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails/nonexistent", "token-alpha")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetMail_DoesNotLeakOtherAgentMail(t *testing.T) {
	env := newTestEnv(t)
	msg := env.seedMessage(t, sampleMessage("<secret@test>", "Beta secret", "bot+beta@example.com", time.Hour))

	// alpha must not see beta's message even with a valid ID
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails/"+msg.ID, "token-alpha")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetMail_Valid(t *testing.T) {
	env := newTestEnv(t)
	msg := env.seedMessage(t, sampleMessage("<mymsg@test>", "My message", "bot+alpha@example.com", time.Hour))

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails/"+msg.ID, "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)

	var mail api.Mail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mail))
	assert.Equal(t, msg.ID, mail.ID)
	assert.Equal(t, "My message", mail.Subject)
	assert.Equal(t, "bot+alpha@example.com", mail.To)
	assert.Equal(t, "body of My message", mail.TextBody)
}
