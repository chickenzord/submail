package api

import (
	"net/http"
	"strings"

	"github.com/chickenzord/submail/internal/config"
	"github.com/labstack/echo/v4"
)

const agentContextKey = "agent"

// agentContext holds the authenticated agent's data for the duration of a request.
type agentContext struct {
	ID        string
	Addresses []string
}

// authMiddleware validates the Bearer token and attaches agent context to the request.
func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, ok := bearerToken(c.Request().Header.Get("Authorization"))
			if !ok || token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or malformed Bearer token")
			}

			agent := s.findAgent(token)
			if agent == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			c.Set(agentContextKey, &agentContext{ID: agent.ID, Addresses: agent.Addresses})
			return next(c)
		}
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header value.
// Returns the token and true on success, empty string and false otherwise.
func bearerToken(header string) (string, bool) {
	prefix, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(prefix, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func (s *Server) findAgent(token string) *config.AgentConfig {
	for i := range s.cfg.Agents {
		if s.cfg.Agents[i].Token == token {
			return &s.cfg.Agents[i]
		}
	}
	return nil
}
