package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MyauDev/vekst/core/classify"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/migrate"
)

// stubDB fakes the subset of *db.DB that /readyz needs, so these tests need
// no real Postgres connection.
type stubDB struct {
	pingErr        error
	appliedVersion int64
	appliedErr     error
}

func (s stubDB) Ping(context.Context) error { return s.pingErr }

func (s stubDB) AppliedMigrationVersion(context.Context) (int64, error) {
	return s.appliedVersion, s.appliedErr
}

// healthyDB reports the exact version this binary requires, so it never
// drifts out of sync with the embedded migrations as they grow.
func healthyDB(t *testing.T) stubDB {
	t.Helper()
	required, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatalf("migrate.RequiredVersion: %v", err)
	}
	return stubDB{appliedVersion: required}
}

func newTestServer(t *testing.T, database readinessChecker) *Server {
	t.Helper()
	cfg := config.Config{Addr: "127.0.0.1:0", ShutdownTimeout: time.Second}
	return New(cfg, discard(), classify.Unavailable{}, database, nil)
}

// Task 6.2: with the database unreachable, /readyz fails and /healthz still
// succeeds -- a database blip must stop traffic, not restart the pod.
func TestReadyzFailsWhenDatabaseUnreachable(t *testing.T) {
	srv := newTestServer(t, stubDB{pingErr: errors.New("connection refused")})
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/readyz status = %d, want %d", res.StatusCode, http.StatusServiceUnavailable)
	}

	res2, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200 even though the database is down", res2.StatusCode)
	}
}

// Task 6.4: with the schema behind the binary's required version, /readyz
// fails and names both the applied and required versions.
func TestReadyzFailsWhenSchemaBehind(t *testing.T) {
	required, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatalf("migrate.RequiredVersion: %v", err)
	}

	srv := newTestServer(t, stubDB{appliedVersion: required - 1})
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, want %d", res.StatusCode, http.StatusServiceUnavailable)
	}

	body := make([]byte, 512)
	n, _ := res.Body.Read(body)
	got := string(body[:n])
	wantApplied := strconv.FormatInt(required-1, 10)
	wantRequired := strconv.FormatInt(required, 10)
	if !strings.Contains(got, wantApplied) || !strings.Contains(got, wantRequired) {
		t.Errorf("/readyz body = %q, want it to name applied %s and required %s", got, wantApplied, wantRequired)
	}
}

// Task 6.1 / happy path: a reachable database at the required schema version
// makes /readyz succeed.
func TestReadyzSucceedsWhenHealthy(t *testing.T) {
	srv := newTestServer(t, healthyDB(t))
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("/readyz status = %d, want 200", res.StatusCode)
	}
}

// Task 6.3: HealthService/Check still returns STATUS_SERVING with no
// database reachable -- 0.1's rule, still true after 0.2 adds Postgres.
func TestHealthServiceIgnoresDatabaseState(t *testing.T) {
	srv := newTestServer(t, stubDB{pingErr: errors.New("connection refused")})
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200 regardless of database state", res.StatusCode)
	}
}
