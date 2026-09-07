package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/MyauDev/vekst/core/classify"
	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/gen/vekst/v1/vektv1connect"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/identity"
)

// authTestServer builds a server with the auth middleware and interceptor in
// place but sign-in unconfigured, which is the state every request below is
// meant to arrive in: anonymous.
func authTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.Config{Addr: "127.0.0.1:0", ShutdownTimeout: time.Second}
	srv := New(cfg, discard(), classify.Unavailable{}, healthyDB(t), nil)
	ts := httptest.NewServer(srv.http.Handler)
	t.Cleanup(ts.Close)
	return ts
}

// Task 8.8 and the platform-foundation delta's "probes are exempt from
// authentication": liveness and readiness answer with no cookie, and carry no
// user data.
func TestProbesAnswerWithoutAuthentication(t *testing.T) {
	ts := authTestServer(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			res, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 -- a probe must never be answered "+
					"with an unauthenticated error", res.StatusCode)
			}
			body, _ := io.ReadAll(res.Body)
			for _, leak := range []string{"@", "user", "email", "session"} {
				if strings.Contains(strings.ToLower(string(body)), leak) {
					t.Errorf("probe response mentions %q; it must identify no person: %s", leak, body)
				}
			}
		})
	}
}

// Task 6.2's trap: HealthService/Check is a Connect RPC behind /rpc rather than
// an HTTP probe path, and change 0.2's spec requires it to answer with no
// database and no credential. Forgetting to exempt it breaks that test, so this
// asserts the exemption directly rather than by side effect.
func TestHealthRPCStaysReachableWithoutASession(t *testing.T) {
	ts := authTestServer(t)

	client := vektv1connect.NewHealthServiceClient(http.DefaultClient, ts.URL+"/rpc")
	resp, err := client.Check(context.Background(), connect.NewRequest(&vektv1.CheckRequest{}))
	if err != nil {
		t.Fatalf("HealthService/Check with no session: %v", err)
	}
	if resp.Msg.GetStatus() != vektv1.CheckResponse_STATUS_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Msg.GetStatus())
	}
}

// The spec's "an anonymous call is rejected": with the unauthenticated code,
// and with no user data in the response.
func TestAuthenticatedRPCIsRejectedWithoutASession(t *testing.T) {
	ts := authTestServer(t)

	client := vektv1connect.NewIdentityServiceClient(http.DefaultClient, ts.URL+"/rpc")
	resp, err := client.GetCurrentUser(context.Background(),
		connect.NewRequest(&vektv1.GetCurrentUserRequest{}))

	if err == nil {
		t.Fatalf("GetCurrentUser with no session succeeded, returning %v", resp.Msg.GetUser())
	}
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", got, connect.CodeUnauthenticated)
	}
	// A code, not a sentence: translation is the client's.
	if !strings.Contains(err.Error(), identity.CodeUnauthenticated) {
		t.Errorf("error %q does not carry the %q code", err, identity.CodeUnauthenticated)
	}
}

// With sign-in unconfigured the auth routes still exist and answer with a
// configuration code, rather than panicking on a nil service. A developer with
// no OAuth client gets a working stack (design D6a).
func TestAuthRoutesAnswerWhenSignInIsNotConfigured(t *testing.T) {
	ts := authTestServer(t)
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	res, err := client.Get(ts.URL + "/auth/google/start")
	if err != nil {
		t.Fatalf("GET /auth/google/start: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); !strings.Contains(loc, identity.CodeNotConfigured) {
		t.Errorf("redirected to %q, want it to carry %q", loc, identity.CodeNotConfigured)
	}
}
