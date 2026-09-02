package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/MyauDev/vekst/core/internal/migrate"
)

// readinessTimeout bounds the database round trips /readyz makes. Short on
// purpose: this is a probe, not a request.
const readinessTimeout = 3 * time.Second

// readinessChecker is the subset of *db.DB that /readyz needs. Defined here,
// not in core/internal/db, so tests can fake it without a real connection.
type readinessChecker interface {
	Ping(ctx context.Context) error
	AppliedMigrationVersion(ctx context.Context) (int64, error)
}

// readyz reports ready only when the process responds, the database pool
// answers, and the applied schema version is at least what this binary
// requires (design D6). Liveness (/healthz) never checks any of this: a
// database blip must stop traffic, not restart the pod.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		notReady(w, "database unreachable: "+err.Error())
		return
	}

	// Derived from the binary's embedded migrations, never a separately
	// declared constant that could disagree with them (design Q5).
	required, err := migrate.RequiredVersion()
	if err != nil {
		notReady(w, "required schema version unknown: "+err.Error())
		return
	}

	applied, err := s.db.AppliedMigrationVersion(ctx)
	if err != nil {
		notReady(w, "applied schema version unknown: "+err.Error())
		return
	}

	if applied < required {
		notReady(w, fmt.Sprintf("schema behind: applied %d, required %d", applied, required))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready\n"))
}

func notReady(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(reason + "\n"))
}
