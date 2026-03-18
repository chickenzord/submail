package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore embeds a real Store and can override specific methods for error testing.
type mockStore struct {
	storage.Store
	countErr    error
	listErr     error
	getErr      error
	countAllErr error
	listAllErr  error
}

func (m *mockStore) CountMessages(ctx context.Context, addrs []string) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.Store.CountMessages(ctx, addrs)
}

func (m *mockStore) ListMessages(ctx context.Context, addrs []string, limit, offset int) ([]*storage.Message, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.Store.ListMessages(ctx, addrs, limit, offset)
}

func (m *mockStore) GetMessage(ctx context.Context, id string) (*storage.Message, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.Store.GetMessage(ctx, id)
}

func (m *mockStore) CountAllMessages(ctx context.Context) (int, error) {
	if m.countAllErr != nil {
		return 0, m.countAllErr
	}
	return m.Store.CountAllMessages(ctx)
}

func (m *mockStore) ListAllMessages(ctx context.Context, limit, offset int) ([]*storage.Message, error) {
	if m.listAllErr != nil {
		return nil, m.listAllErr
	}
	return m.Store.ListAllMessages(ctx, limit, offset)
}

// newMockEnv builds a test environment backed by a mock store.
func newMockEnv(t *testing.T, mock *mockStore) *testEnv {
	t.Helper()
	real, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { real.Close() })
	mock.Store = real
	cfg := &config.Config{Agents: testAgents}
	return &testEnv{srv: api.NewServer(cfg, mock), store: mock, cfg: cfg}
}

// newAdminMockEnv builds a test environment with admin enabled backed by a mock store.
func newAdminMockEnv(t *testing.T, mock *mockStore) *testEnv {
	t.Helper()
	real, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { real.Close() })
	mock.Store = real
	cfg := &config.Config{
		Agents: testAgents,
		Server: config.ServerConfig{
			Admin: config.AdminConfig{Enabled: true, Password: "adminpass"},
		},
	}
	return &testEnv{srv: api.NewServer(cfg, mock), store: mock, cfg: cfg}
}

// --- Store error path tests ---

func TestListMails_CountError(t *testing.T) {
	mock := &mockStore{countErr: errors.New("db down")}
	env := newMockEnv(t, mock)

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-alpha")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestListMails_ListError(t *testing.T) {
	mock := &mockStore{listErr: errors.New("db down")}
	env := newMockEnv(t, mock)

	rec := env.do(http.MethodGet, "/api/v1/inbox/mails", "token-alpha")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGetMail_StoreError(t *testing.T) {
	mock := &mockStore{getErr: errors.New("db down")}
	env := newMockEnv(t, mock)

	// Seed a real message first
	msg := env.seedMessage(t, sampleMessage("<test@msg>", "test", "bot+alpha@example.com", 0))

	// Now set the error and try to retrieve
	rec := env.do(http.MethodGet, "/api/v1/inbox/mails/"+msg.ID, "token-alpha")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- Admin error path tests ---

func TestAdminListMails_CountAllError(t *testing.T) {
	mock := &mockStore{countAllErr: errors.New("db down")}
	env := newAdminMockEnv(t, mock)
	rec := doAdmin(env, http.MethodGet, "/admin/")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestAdminListMails_ListAllError(t *testing.T) {
	mock := &mockStore{listAllErr: errors.New("db down")}
	env := newAdminMockEnv(t, mock)
	rec := doAdmin(env, http.MethodGet, "/admin/")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestAdminGetMail_StoreError(t *testing.T) {
	mock := &mockStore{}
	env := newAdminMockEnv(t, mock)
	msg := env.seedMessage(t, sampleMessage("<admin-err@test>", "Admin err", "bot+alpha@example.com", 0))
	mock.getErr = errors.New("db down")
	rec := doAdmin(env, http.MethodGet, "/admin/mails/"+msg.ID)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
