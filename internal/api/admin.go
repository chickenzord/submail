package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const adminCookieName = "_submail_admin"

//go:embed templates
var templateFS embed.FS

var adminTemplates *template.Template

func init() {
	adminTemplates = template.Must(
		template.New("").
			Funcs(template.FuncMap{
				"timeFormat": func(t time.Time) string {
					return t.UTC().Format("2006-01-02 15:04:05 UTC")
				},
				"timeRFC3339": func(t time.Time) string {
					return t.UTC().Format(time.RFC3339)
				},
				"gravatarURL": func(email string) string {
					if addr, err := mail.ParseAddress(email); err == nil {
						email = addr.Address
					}
					email = strings.ToLower(strings.TrimSpace(email))
					h := sha256.Sum256([]byte(email))
					return fmt.Sprintf("https://www.gravatar.com/avatar/%x?s=36&d=identicon", h)
				},
			}).
			ParseFS(templateFS, "templates/*.html"),
	)
}

// adminSessionToken returns the expected cookie value for the given admin password.
// It is derived via HMAC-SHA256 so that:
//   - the raw password is never stored in the cookie, and
//   - changing the password immediately invalidates all existing sessions.
func adminSessionToken(password string) string {
	h := hmac.New(sha256.New, []byte("submail-admin-session-v1"))
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// adminSessionMiddleware rejects requests that do not carry a valid session
// cookie and redirects them to the login page.
func (s *Server) adminSessionMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie, err := c.Cookie(adminCookieName)
			expected := adminSessionToken(s.cfg.Server.Admin.Password)
			if err != nil || !hmac.Equal([]byte(cookie.Value), []byte(expected)) {
				return c.Redirect(http.StatusFound, "/admin/login")
			}
			return next(c)
		}
	}
}

// renderAdmin writes an HTML response using the named template.
func renderAdmin(c echo.Context, name string, data any) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return adminTemplates.ExecuteTemplate(c.Response().Writer, name, data)
}
