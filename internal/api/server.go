package api

import (
	"context"
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

	if s.cfg.Server.Admin.Enabled {
		s.echo.GET("/", func(c echo.Context) error {
			return c.Redirect(http.StatusFound, "/admin/")
		})

		admin := s.echo.Group("/admin")
		// Public: login / logout (no session required)
		admin.GET("/login", s.adminLoginForm)
		admin.POST("/login", s.adminLoginSubmit)
		admin.GET("/logout", s.adminLogout)
		// Protected: everything else requires a valid session cookie
		secured := admin.Group("", s.adminSessionMiddleware())
		secured.GET("", s.adminListMails)
		secured.GET("/", s.adminListMails)
		secured.GET("/mails/:id", s.adminGetMail)
	}
}

// Handler returns the underlying http.Handler, useful for testing.
func (s *Server) Handler() http.Handler {
	return s.echo
}

// Start begins listening on addr. It returns http.ErrServerClosed on graceful shutdown.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// Shutdown gracefully stops the server, waiting up to the deadline in ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
