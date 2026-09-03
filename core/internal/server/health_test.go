package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/MyauDev/vekst/core/classify"
	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/gen/vekst/v1/vektv1connect"
	"github.com/MyauDev/vekst/core/internal/config"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubClassifier struct {
	info  classify.VersionInfo
	err   error
	delay time.Duration
}

func (s stubClassifier) Version(ctx context.Context) (classify.VersionInfo, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return classify.VersionInfo{}, ctx.Err()
		}
	}
	return s.info, s.err
}

// Task 5.1: Check reports serving, with a version and an RFC 3339 build time.
func TestCheckReportsBuildIdentity(t *testing.T) {
	h := &healthHandler{classifier: stubClassifier{info: classify.VersionInfo{EngineVersion: "engine-1"}}, log: discard()}

	resp, err := h.Check(context.Background(), connect.NewRequest(&vektv1.CheckRequest{}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	msg := resp.Msg
	if msg.GetStatus() != vektv1.CheckResponse_STATUS_SERVING {
		t.Errorf("status = %v, want STATUS_SERVING", msg.GetStatus())
	}
	if msg.GetVersion() == "" {
		t.Error("version is empty; an unstamped build must report \"dev\", never nothing")
	}
	if msg.GetBuiltAt() == "" {
		t.Error("built_at is empty")
	}
	if got := msg.GetClassifierVersion(); got != "engine-1" {
		t.Errorf("classifier_version = %q, want %q", got, "engine-1")
	}
}

// Task 5.4: with the classifier unreachable, core still serves. A classifier
// outage is a retryable condition, not an outage of core (ARCHITECTURE.md 3.5).
func TestCheckDegradesWhenClassifierUnavailable(t *testing.T) {
	for name, c := range map[string]classify.Classifier{
		"not configured": classify.Unavailable{},
		"rpc error":      stubClassifier{err: errors.New("connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			h := &healthHandler{classifier: c, log: discard()}

			resp, err := h.Check(context.Background(), connect.NewRequest(&vektv1.CheckRequest{}))
			if err != nil {
				t.Fatalf("Check returned an error; a classifier outage must not fail core: %v", err)
			}
			if resp.Msg.GetStatus() != vektv1.CheckResponse_STATUS_SERVING {
				t.Errorf("status = %v, want STATUS_SERVING", resp.Msg.GetStatus())
			}
			if got := resp.Msg.GetClassifierVersion(); got != "" {
				t.Errorf("classifier_version = %q, want empty", got)
			}
		})
	}
}

// Task 5.4 (second half): the call is bounded. A slow classifier must not make
// the caller wait past the deadline.
func TestCheckRespectsDeadline(t *testing.T) {
	h := &healthHandler{classifier: stubClassifier{delay: time.Hour}, log: discard()}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	resp, err := h.Check(ctx, connect.NewRequest(&vektv1.CheckRequest{}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %s; it must not outlive its deadline", elapsed)
	}
	if resp.Msg.GetClassifierVersion() != "" {
		t.Error("classifier_version should be empty when the call timed out")
	}
}

// Task 5.2: the process serves with no database. Change 0.2 adds Postgres; this
// asserts the constraint that survives it -- liveness never depends on it.
func TestServesWithNoDatabase(t *testing.T) {
	cfg := config.Config{Addr: "127.0.0.1:0", ShutdownTimeout: time.Second}
	srv := New(cfg, discard(), classify.Unavailable{}, healthyDB(t))

	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	t.Run("healthz", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", res.StatusCode)
		}
	})

	t.Run("rpc", func(t *testing.T) {
		client := vektv1connect.NewHealthServiceClient(http.DefaultClient, ts.URL+"/rpc")
		resp, err := client.Check(context.Background(), connect.NewRequest(&vektv1.CheckRequest{}))
		if err != nil {
			t.Fatalf("Check over the wire: %v", err)
		}
		if resp.Msg.GetStatus() != vektv1.CheckResponse_STATUS_SERVING {
			t.Errorf("status = %v, want STATUS_SERVING", resp.Msg.GetStatus())
		}
	})
}

// Run must serve until its context is cancelled, then drain and return without
// error. A shutdown that reports failure would make every rolling deploy look
// like an incident.
func TestRunServesThenShutsDownCleanly(t *testing.T) {
	cfg := config.Config{Addr: "127.0.0.1:0", ShutdownTimeout: 2 * time.Second}
	srv := New(cfg, discard(), classify.Unavailable{}, healthyDB(t))

	// Bind an ephemeral port ourselves so the test never collides with a
	// developer's running stack.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.http.Addr = ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Wait for the listener to accept.
	deadline := time.Now().Add(3 * time.Second)
	var up bool
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", srv.http.Addr, 100*time.Millisecond); err == nil {
			_ = c.Close()
			up = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !up {
		cancel()
		t.Fatalf("server never accepted a connection on %s", srv.http.Addr)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil after a graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// A port already in use must surface as an error, not a silent no-op.
func TestRunReportsListenFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	cfg := config.Config{Addr: ln.Addr().String(), ShutdownTimeout: time.Second}
	srv := New(cfg, discard(), classify.Unavailable{}, healthyDB(t))

	if err := srv.Run(context.Background()); err == nil {
		t.Error("Run returned nil when the port was already bound")
	}
}
