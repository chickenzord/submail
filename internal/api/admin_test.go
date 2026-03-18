package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAdminTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	cfg := &config.Config{
		Agents: testAgents,
		Server: config.ServerConfig{
			Admin: config.AdminConfig{Enabled: true, Password: "adminpass"},
		},
	}
	return &testEnv{srv: api.NewServer(cfg, store), store: store, cfg: cfg}
}

// validAdminCookie returns the correct session cookie value for "adminpass".
func validAdminCookie() string {
	return api.AdminSessionToken("adminpass")
}

// doAdmin makes a request with the admin session cookie pre-set.
func doAdmin(env *testEnv, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: "_submail_admin", Value: validAdminCookie()})
	rec := httptest.NewRecorder()
	env.srv.Handler().ServeHTTP(rec, req)
	return rec
}

// doForm posts form-encoded data.
func doForm(env *testEnv, path string, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	env.srv.Handler().ServeHTTP(rec, req)
	return rec
}

// --- Admin tests ---

func TestAdmin_RootRedirectsToAdmin(t *testing.T) {
	env := newAdminTestEnv(t)
	rec := doAdmin(env, http.MethodGet, "/")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/admin/")
}

func TestAdmin_LoginForm(t *testing.T) {
	env := newAdminTestEnv(t)
	rec := doAdmin(env, http.MethodGet, "/admin/login")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<form")
}

func TestAdmin_LoginSubmit_WrongPassword(t *testing.T) {
	env := newAdminTestEnv(t)
	values := url.Values{"password": []string{"wrongpass"}}
	rec := doForm(env, "/admin/login", values)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid password")
}

func TestAdmin_LoginSubmit_CorrectPassword(t *testing.T) {
	env := newAdminTestEnv(t)
	values := url.Values{"password": []string{"adminpass"}}
	rec := doForm(env, "/admin/login", values)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/admin/", rec.Header().Get("Location"))

	// Check that Set-Cookie header is present and contains the admin cookie
	setCookie := rec.Header().Get("Set-Cookie")
	assert.NotEmpty(t, setCookie)
	assert.Contains(t, setCookie, "_submail_admin")
}

func TestAdmin_Session_NoCookie(t *testing.T) {
	env := newAdminTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	env.srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/admin/login")
}

func TestAdmin_Session_WrongCookie(t *testing.T) {
	env := newAdminTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "_submail_admin", Value: "badvalue"})
	rec := httptest.NewRecorder()
	env.srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/admin/login")
}

func TestAdmin_Session_ValidCookie(t *testing.T) {
	env := newAdminTestEnv(t)
	rec := doAdmin(env, http.MethodGet, "/admin/")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAdmin_Logout(t *testing.T) {
	env := newAdminTestEnv(t)
	rec := doAdmin(env, http.MethodGet, "/admin/logout")

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/admin/login")

	// Check that Set-Cookie header clears the cookie (empty value and old expires)
	setCookie := rec.Header().Get("Set-Cookie")
	assert.NotEmpty(t, setCookie)
	assert.Contains(t, setCookie, "_submail_admin=")
	// Cookie should have either MaxAge=0 or an old Expires date
	assert.True(t, 
		strings.Contains(setCookie, "Max-Age=0") || strings.Contains(setCookie, "Expires=Thu, 01 Jan 1970"),
		"cookie should be cleared with old expires or zero max-age")
}

func TestAdmin_ListMails(t *testing.T) {
	env := newAdminTestEnv(t)
	env.seedMessage(t, sampleMessage("<msg1@test>", "Test Subject", "bot+alpha@example.com", time.Hour))

	rec := doAdmin(env, http.MethodGet, "/admin/")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Test Subject")
}

func TestAdmin_GetMail_Found(t *testing.T) {
	env := newAdminTestEnv(t)
	msg := env.seedMessage(t, sampleMessage("<msg2@test>", "Detail Subject", "bot+alpha@example.com", time.Hour))

	rec := doAdmin(env, http.MethodGet, "/admin/mails/"+msg.ID)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Detail Subject")
}

func TestAdmin_GetMail_NotFound(t *testing.T) {
	env := newAdminTestEnv(t)
	rec := doAdmin(env, http.MethodGet, "/admin/mails/nope")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdmin_RoutesNotRegistered_WhenDisabled(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/admin/login", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
