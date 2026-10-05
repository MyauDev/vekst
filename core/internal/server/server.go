// Package server wires the HTTP surface: chi for plain endpoints, connect-go
// for the browser API.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MyauDev/vekst/core/classify"
	"github.com/MyauDev/vekst/core/gen/vekst/v1/vektv1connect"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/ingest"
	"github.com/MyauDev/vekst/core/internal/report"
	"github.com/MyauDev/vekst/core/internal/review"
	"github.com/MyauDev/vekst/core/internal/tenancy"
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
func New(cfg config.Config, log *slog.Logger, classifier classify.Classifier, database readinessChecker, ident *identity.Service, importSvc *ingest.Service,
	reviewer *review.Service, reporter *report.Service, tenant *tenancy.Service) *Server {
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

	// Resolve the session for every request, and reject nothing here: the
	// probes and the auth routes must answer without one, and what requires a
	// session is the interceptor's decision. A nil ident (sign-in not
	// configured) passes everything through anonymously.
	r.Use(ident.Middleware)

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

	// The OIDC redirect flow: three plain HTTP routes, because a Connect
	// handler cannot answer with a 302 and the callback arrives as a browser
	// navigation. The one exception to "the browser talks to core over
	// Connect", and one about transport rather than contract -- no application
	// data crosses them (add-identity design D1).
	ident.Routes(r)

	// The browser API. Connect over HTTP, mounted under /rpc so the Ingress can
	// route by path prefix and the browser stays same-origin (design D6).
	//
	// HealthService/Check is exempt from authentication: it is a Connect RPC
	// behind /rpc rather than an HTTP probe path, and change 0.2's spec
	// requires it to answer with no database and no credential.
	authOpt := connect.WithInterceptors(identity.NewInterceptor(
		vektv1connect.HealthServiceCheckProcedure,
	))

	path, handler := vektv1connect.NewHealthServiceHandler(
		&healthHandler{classifier: classifier, log: log}, authOpt)
	r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))

	path, handler = vektv1connect.NewIdentityServiceHandler(&identityHandler{svc: ident}, authOpt)
	r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))

	path, handler = vektv1connect.NewImportServiceHandler(&importHandler{svc: importSvc}, authOpt)
	r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))

	// OrgService, on the same terms as every other handler -- authenticated,
	// nothing more. CreateOrganization is the one RPC a caller with a session
	// and no membership may reach; a nil tenant is what the tests exercising
	// only health and identity pass, the same shape reviewer and reporter
	// already use below.
	if tenant != nil {
		path, handler = vektv1connect.NewOrgServiceHandler(&orgHandler{svc: tenant}, authOpt)
		r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))
	}

	// The review queue. Registered only with a service, because every one of
	// its calls opens a transaction bound to an organisation and there is
	// nothing useful it can answer without one. A nil reviewer is what the
	// tests exercising only health and identity pass.
	if reviewer != nil {
		path, handler = vektv1connect.NewReviewServiceHandler(&reviewHandler{svc: reviewer}, authOpt)
		r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))
	}

	// The management P&L, on the same terms and for the same reason.
	if reporter != nil {
		path, handler = vektv1connect.NewReportServiceHandler(&reportHandler{svc: reporter}, authOpt)
		r.Mount("/rpc"+path, http.StripPrefix("/rpc", handler))
	}

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
