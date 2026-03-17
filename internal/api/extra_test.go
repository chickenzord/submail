package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- listMails edge cases ---

func TestListMails_LimitCappedAt200(t *testing.T) {
	env := newTestEnv(t)
	env.seedMessage(t, sampleMessage("<cap@test>", "Cap test", "bot+alpha@example.com", time.Hour))

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails?limit=999", "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 200, resp.Limit)
}

func TestListMails_InvalidLimitUsesDefault(t *testing.T) {
	env := newTestEnv(t)
	env.seedMessage(t, sampleMessage("<inv@test>", "Invalid limit", "bot+alpha@example.com", time.Hour))

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails?limit=abc", "token-alpha")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 50, resp.Limit)
}

// --- multiple addresses per agent ---

func TestListMails_MultipleAddressesPerAgent(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	multiAgent := config.AgentConfig{
		ID:        "multi",
		Token:     "token-multi",
		Addresses: []string{"bot+x@example.com", "bot+y@example.com"},
	}
	cfg := &config.Config{Agents: []config.AgentConfig{multiAgent}}
	env := &testEnv{srv: api.NewServer(cfg, store), store: store, cfg: cfg}

	env.seedMessage(t, sampleMessage("<mx@test>", "To X", "bot+x@example.com", time.Hour))
	env.seedMessage(t, sampleMessage("<my@test>", "To Y", "bot+y@example.com", 2*time.Hour))

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-multi")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ListMailsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Mails, 2)
}
