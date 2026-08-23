package classify_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/MyauDev/vekst/core/classify"
	internalv1 "github.com/MyauDev/vekst/core/gen/vekstinternal/v1"
)

// stubServer implements the real generated service interface, so this exercises
// the actual wire format rather than a hand-written fake of it.
type stubServer struct {
	internalv1.UnimplementedClassifierServiceServer
	version string
	err     error
	delay   time.Duration
}

func (s *stubServer) Version(ctx context.Context, _ *internalv1.VersionRequest) (*internalv1.VersionResponse, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return &internalv1.VersionResponse{EngineVersion: s.version, BuiltAt: "2026-08-23T12:00:00Z"}, nil
}

// serve starts a real gRPC server on a loopback port and returns its address.
func serve(t *testing.T, impl *stubServer) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	internalv1.RegisterClassifierServiceServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

func TestGRPCClientReportsVersion(t *testing.T) {
	addr := serve(t, &stubServer{version: "engine-1"})

	c, err := classify.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	info, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if info.EngineVersion != "engine-1" {
		t.Errorf("EngineVersion = %q, want %q", info.EngineVersion, "engine-1")
	}
	if info.BuiltAt == "" {
		t.Error("BuiltAt is empty")
	}
}

// ARCHITECTURE.md 3.5: a classifier that is not answering is a retryable
// condition. The client must surface it as an error and not hang.
func TestGRPCClientErrorsWhenServerIsDown(t *testing.T) {
	addr := serve(t, &stubServer{})
	c, err := classify.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	// A port nobody is listening on.
	dead, err := classify.Dial("127.0.0.1:1")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = dead.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := dead.Version(ctx); err == nil {
		t.Error("Version() succeeded against a dead address")
	}
}

// Dialing is lazy, so a classifier that has not started yet must not stop core
// from starting.
func TestDialDoesNotConnectEagerly(t *testing.T) {
	c, err := classify.Dial("127.0.0.1:1")
	if err != nil {
		t.Fatalf("Dial against a dead address should not fail: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestGRPCClientRespectsDeadline(t *testing.T) {
	addr := serve(t, &stubServer{delay: time.Hour})
	c, err := classify.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := c.Version(ctx); err == nil {
		t.Error("Version() ignored its deadline")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Version() took %s; it must honour the caller's deadline", elapsed)
	}
}

// Unavailable is what core uses when no classifier address is configured. It
// exists so the "classifier is unreachable" path is the same code everywhere,
// rather than a nil check only the tests exercise.
func TestUnavailableAlwaysFails(t *testing.T) {
	var c classify.Classifier = classify.Unavailable{}

	info, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("Unavailable.Version() returned no error")
	}
	if info != (classify.VersionInfo{}) {
		t.Errorf("Unavailable.Version() returned %+v, want the zero value", info)
	}
	if errors.Is(err, context.Canceled) {
		t.Error("error should describe the missing configuration, not a cancellation")
	}
}
