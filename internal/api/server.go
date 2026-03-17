package api

import (
	"net/http"

	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server wraps an Echo instance with app dependencies.
type Server struct {
	echo  *echo.Echo
	cfg   *config.Config
	store storage.Store
}

// NewServer creates and configures a new Server.
func NewServer(cfg *config.Config, store storage.Store) *Server {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	s := &Server{echo: e, cfg: cfg, store: store}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	v1 := s.echo.Group("/api/v1")
	inbox := v1.Group("/inbox", s.authMiddleware())
	inbox.GET("/mails", s.listMails)
	inbox.GET("/mails/:id", s.getMail)
}

// Handler returns the underlying http.Handler, useful for testing.
func (s *Server) Handler() http.Handler {
	return s.echo
}

// Start begins listening on addr.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
