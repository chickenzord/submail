package api

import (
	"crypto/hmac"
	"net/http"
	"strconv"
	"time"

	"github.com/chickenzord/submail/internal/storage"
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

type adminAgentInfo struct {
	ID        string
	Addresses []string
	Active    bool
}

type adminListData struct {
	Mails       []*Mail
	Agents      []adminAgentInfo
	AgentFilter string // selected agent ID; empty = all
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

// adminListMails handles GET /admin/ — lists all messages across all inboxes,
// with optional per-agent filtering via ?agent=<id>.
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

	// Resolve agent filter: build agent list and find filter addresses if set.
	agentFilter := c.QueryParam("agent")
	agents := make([]adminAgentInfo, len(s.cfg.Agents))
	var filterAddresses []string
	for i, a := range s.cfg.Agents {
		active := a.ID == agentFilter
		agents[i] = adminAgentInfo{ID: a.ID, Addresses: a.Addresses, Active: active}
		if active {
			filterAddresses = a.Addresses
		}
	}
	// Unknown agent ID → treat as "all"
	if agentFilter != "" && filterAddresses == nil {
		agentFilter = ""
	}

	ctx := c.Request().Context()

	var total int
	var err error
	if len(filterAddresses) > 0 {
		total, err = s.store.CountMessages(ctx, filterAddresses)
	} else {
		total, err = s.store.CountAllMessages(ctx)
	}
	if err != nil {
		c.Logger().Errorf("admin: count messages: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count messages")
	}

	var storeMsgs []*storage.Message
	if len(filterAddresses) > 0 {
		storeMsgs, err = s.store.ListMessages(ctx, filterAddresses, limit, offset)
	} else {
		storeMsgs, err = s.store.ListAllMessages(ctx, limit, offset)
	}
	if err != nil {
		c.Logger().Errorf("admin: list messages: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list messages")
	}

	mails := make([]*Mail, len(storeMsgs))
	for i, m := range storeMsgs {
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
		Agents:      agents,
		AgentFilter: agentFilter,
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
	ReceivedAt time.Time
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
