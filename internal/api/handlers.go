package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// listMails handles GET /inbox
func (s *Server) listMails(c echo.Context) error {
	agent := c.Get(agentContextKey).(*agentContext)

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

	total, err := s.store.CountMessages(ctx, agent.Addresses)
	if err != nil {
		c.Logger().Errorf("count mails for agent %q: %v", agent.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count mails")
	}

	messages, err := s.store.ListMessages(ctx, agent.Addresses, limit, offset)
	if err != nil {
		c.Logger().Errorf("list mails for agent %q: %v", agent.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list mails")
	}

	mails := make([]*Mail, len(messages))
	for i, m := range messages {
		mails[i] = mailFromMessage(m)
	}

	return c.JSON(http.StatusOK, ListMailsResponse{
		Mails:  mails,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// getMail handles GET /inbox/:id
func (s *Server) getMail(c echo.Context) error {
	agent := c.Get(agentContextKey).(*agentContext)
	id := c.Param("id")

	msg, err := s.store.GetMessage(c.Request().Context(), id)
	if err != nil {
		c.Logger().Errorf("get mail %q for agent %q: %v", id, agent.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get mail")
	}
	if msg == nil || !containsAddr(agent.Addresses, msg.To) {
		// Return 404 whether not found or not owned — don't leak existence
		return echo.NewHTTPError(http.StatusNotFound, "mail not found")
	}

	return c.JSON(http.StatusOK, mailFromMessage(msg))
}

func containsAddr(addresses []string, addr string) bool {
	for _, a := range addresses {
		if a == addr {
			return true
		}
	}
	return false
}
