package api

import (
	"crypto/hmac"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// adminLoginForm handles GET /admin/login — shows the login page.
func (s *Server) adminLoginForm(c echo.Context) error {
	return renderAdmin(c, "login.html", struct{ Error string }{})
}

// adminLoginSubmit handles POST /admin/login — verifies the password and sets
// a session cookie on success, or re-renders the login form with an error.
func (s *Server) adminLoginSubmit(c echo.Context) error {
	password := c.FormValue("password")
	expected := s.cfg.Server.Admin.Password

	// constant-time comparison via HMAC equality on equal-length tokens
	if !hmac.Equal([]byte(adminSessionToken(password)), []byte(adminSessionToken(expected))) {
		return renderAdmin(c, "login.html", struct{ Error string }{"Invalid password."})
	}

	cookie := new(http.Cookie)
	cookie.Name = adminCookieName
	cookie.Value = adminSessionToken(expected)
	cookie.Path = "/admin"
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Expires = time.Now().Add(24 * time.Hour)
	c.SetCookie(cookie)

	return c.Redirect(http.StatusFound, "/admin/")
}

// adminLogout handles GET /admin/logout — clears the session cookie.
func (s *Server) adminLogout(c echo.Context) error {
	cookie := new(http.Cookie)
	cookie.Name = adminCookieName
	cookie.Value = ""
	cookie.Path = "/admin"
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Expires = time.Unix(0, 0)
	cookie.MaxAge = -1
	c.SetCookie(cookie)

	return c.Redirect(http.StatusFound, "/admin/login")
}

type adminListData struct {
	Mails       []*Mail
	Total       int
	Limit       int
	Offset      int
	OffsetStart int
	OffsetEnd   int
	HasPrev     bool
	HasNext     bool
	PrevOffset  int
	NextOffset  int
}

// adminListMails handles GET /admin/ — lists all messages across all inboxes.
func (s *Server) adminListMails(c echo.Context) error {
	limit := 50
	offset := 0

	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 200)
		}
	}
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	ctx := c.Request().Context()

	total, err := s.store.CountAllMessages(ctx)
	if err != nil {
		c.Logger().Errorf("admin: count all messages: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count messages")
	}

	messages, err := s.store.ListAllMessages(ctx, limit, offset)
	if err != nil {
		c.Logger().Errorf("admin: list all messages: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list messages")
	}

	mails := make([]*Mail, len(messages))
	for i, m := range messages {
		mails[i] = mailFromMessage(m)
	}

	offsetStart := offset + 1
	offsetEnd := offset + len(mails)
	if total == 0 {
		offsetStart = 0
		offsetEnd = 0
	}

	data := adminListData{
		Mails:       mails,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		OffsetStart: offsetStart,
		OffsetEnd:   offsetEnd,
		HasPrev:     offset > 0,
		HasNext:     offset+limit < total,
		PrevOffset:  max(offset-limit, 0),
		NextOffset:  offset + limit,
	}

	return renderAdmin(c, "list.html", data)
}

type adminDetailData struct {
	ID         string
	Subject    string
	From       string
	To         string
	ReceivedAt interface{}
	TextBody   string
	HTMLBody   string
}

// adminGetMail handles GET /admin/mails/:id — shows a single message.
func (s *Server) adminGetMail(c echo.Context) error {
	id := c.Param("id")

	msg, err := s.store.GetMessage(c.Request().Context(), id)
	if err != nil {
		c.Logger().Errorf("admin: get message %q: %v", id, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get message")
	}
	if msg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "message not found")
	}

	data := adminDetailData{
		ID:         msg.ID,
		Subject:    msg.Subject,
		From:       msg.From,
		To:         msg.To,
		ReceivedAt: msg.ReceivedAt,
		TextBody:   msg.TextBody,
		HTMLBody:   msg.HTMLBody,
	}

	return renderAdmin(c, "detail.html", data)
}
