// Package server wires the HTTP surface: chi for plain endpoints, connect-go
// for the browser API.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MyauDev/vekst/core/classify"
	"github.com/MyauDev/vekst/core/gen/vekst/v1/vektv1connect"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
	"github.com/MyauDev/vekst/core/internal/config"
)

// Server owns the HTTP listener and its lifecycle.
type Server struct {
	http *http.Server
	log  *slog.Logger
	cfg  config.Config
	db   readinessChecker
}

// New builds the router and the HTTP server. It performs no I/O. database is
// never nil from change 0.2 onward: core always connects to Postgres.
func New(cfg config.Config, log *slog.Logger, classifier classify.Classifier, database readinessChecker) *Server {
	s := &Server{
		http: &http.Server{
			Addr:              cfg.Addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		log: log,
		cfg: cfg,
		db:  database,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	// Liveness. Deliberately dependency-free: it answers "is this process
	// broken", not "is the system healthy". A liveness probe that checks a
	// dependency turns a recoverable outage into a crash loop.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("live-update-probe " + buildinfo.Version() + "\n"))
	})

	// Readiness. Checks the database and the schema version -- design D6.
	r.Get("/readyz", s.readyz)

	// The browser API. Connect over HTTP, mounted under /rpc so the Ingress can
	// route by path prefix and the browser stays same-origin (design D6).
	path, handler := vektv1connect.NewHealthServiceHandler(&healthHandler{classifier: classifier, log: log})
	r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))

	s.http.Handler = r
	return s
}

// Run serves until ctx is cancelled, then drains within ShutdownTimeout.
func (s *Server) Run(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() {
		s.log.Info("listening", "addr", s.cfg.Addr, "version", buildinfo.Version())
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down", "timeout", s.cfg.ShutdownTimeout)
		// A bounded drain: in-flight requests finish, new ones are refused, and
		// the process exits even if something refuses to let go.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.ShutdownTimeout)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}
