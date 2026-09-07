package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- the happy path, driven through the real redirect chain -----------------

// Task 8.9: the whole chain against a stub provider, so the flow cookie is
// proved to arrive on the callback rather than asserted from its attribute
// string.
func TestFirstSignInCreatesAccountAndSession(t *testing.T) {
	h := newHarness(t)
	c := h.client()

	resp := h.signIn(c, nil)
	defer resp.Body.Close()

	if got := authError(t, resp); got != "" {
		t.Fatalf("sign-in failed with %q, want success", got)
	}
	if loc := resp.Header.Get("Location"); loc != postSignInPath {
		t.Errorf("redirected to %q, want %q", loc, postSignInPath)
	}
	if n := h.countUsers(); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}

	status, body := h.whoami(c)
	if status != http.StatusOK {
		t.Fatalf("/whoami after sign-in: status %d body %q, want 200", status, body)
	}
}

// Task 8.4 and the spec's "a later sign-in reuses the account".
func TestSecondSignInReusesTheAccount(t *testing.T) {
	h := newHarness(t)

	first := h.signIn(h.client(), nil)
	first.Body.Close()
	_, firstID := h.whoami(h.client()) // no session on a fresh client
	_ = firstID

	second := h.signIn(h.client(), nil)
	defer second.Body.Close()
	if got := authError(t, second); got != "" {
		t.Fatalf("second sign-in failed with %q", got)
	}

	if n := h.countUsers(); n != 1 {
		t.Errorf("users = %d after two sign-ins with one subject, want 1", n)
	}
}

// --- token validation: the design's highest risk row ------------------------

// Task 8.1. A decoder would accept this token; a verifier must not.
func TestIDTokenWithBadSignatureIsRefused(t *testing.T) {
	h := newHarness(t)

	resp := h.signIn(h.client(), func(string) { h.provider.signWrong = true })
	if got := authError(t, resp); got != CodeInvalidToken {
		t.Fatalf("auth_error = %q, want %q", got, CodeInvalidToken)
	}
	if n := h.countUsers(); n != 0 {
		t.Errorf("users = %d after a refused sign-in, want 0", n)
	}
}

// Task 8.2: wrong audience, expired, and a nonce that does not match the flow
// -- one case each.
func TestIDTokenValidationRefusesEachBadClaim(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tweak func(h *harness)
	}{
		{"wrong audience", func(h *harness) { h.provider.audience = "someone-elses-client" }},
		{"expired", func(h *harness) { h.provider.lifetime = -time.Minute }},
		{"nonce does not match the flow", func(h *harness) { h.provider.nonce = "not-the-flows-nonce" }},
		{"wrong issuer", func(h *harness) { h.provider.issuerClaim = "https://issuer.example.test" }},
		{"no id_token at all", func(h *harness) { h.provider.omitIDToken = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			resp := h.signIn(h.client(), func(string) { tc.tweak(h) })
			if got := authError(t, resp); got != CodeInvalidToken {
				t.Fatalf("auth_error = %q, want %q", got, CodeInvalidToken)
			}
			if n := h.countUsers(); n != 0 {
				t.Errorf("users = %d, want 0 -- no account may exist for a refused token", n)
			}
			h.assertNoSessions()
		})
	}
}

// --- the flow: both halves are checked --------------------------------------

// Task 8.3.
func TestCallbackWithUnknownOrConsumedStateIsRefused(t *testing.T) {
	h := newHarness(t)

	t.Run("unknown state", func(t *testing.T) {
		resp := h.callback(h.client(), "a-state-nobody-issued", "code")
		if got := authError(t, resp); got != CodeInvalidFlow {
			t.Fatalf("auth_error = %q, want %q", got, CodeInvalidFlow)
		}
	})

	t.Run("consumed state", func(t *testing.T) {
		h.cleanTables()
		c := h.client()

		start, err := c.Get(h.server.URL + "/auth/google/start")
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		start.Body.Close()
		authURL := start.Header.Get("Location")
		h.provider.nonce = nonceFromAuthURL(t, authURL)
		state := stateFromAuthURL(t, authURL)

		first := h.callback(c, state, "code")
		first.Body.Close()
		if got := authError(t, first); got != "" {
			t.Fatalf("first callback failed with %q", got)
		}

		// Replaying the same callback finds nothing: consuming deleted the row.
		second := h.callback(c, state, "code")
		if got := authError(t, second); got != CodeInvalidFlow {
			t.Fatalf("replayed callback: auth_error = %q, want %q", got, CodeInvalidFlow)
		}
	})
}

// Task 8.3b: state alone is not enough. The flow cookie proves the callback
// reached the browser that started the flow.
func TestCallbackWithoutMatchingFlowCookieIsRefused(t *testing.T) {
	h := newHarness(t)

	starter := h.client()
	start, err := starter.Get(h.server.URL + "/auth/google/start")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	start.Body.Close()
	authURL := start.Header.Get("Location")
	h.provider.nonce = nonceFromAuthURL(t, authURL)
	state := stateFromAuthURL(t, authURL)

	t.Run("no flow cookie", func(t *testing.T) {
		// A different browser: same state, no cookie.
		resp := h.callback(h.client(), state, "code")
		if got := authError(t, resp); got != CodeInvalidFlow {
			t.Fatalf("auth_error = %q, want %q", got, CodeInvalidFlow)
		}
		h.assertNoSessions()
	})

	t.Run("cookie naming a different flow", func(t *testing.T) {
		h.cleanTables()

		c := h.client()
		s1, err := c.Get(h.server.URL + "/auth/google/start")
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		s1.Body.Close()
		firstState := stateFromAuthURL(t, s1.Header.Get("Location"))

		// A second start overwrites the cookie with the newer flow's id, so the
		// older state no longer matches what the browser holds.
		s2, err := c.Get(h.server.URL + "/auth/google/start")
		if err != nil {
			t.Fatalf("second start: %v", err)
		}
		s2.Body.Close()
		h.provider.nonce = nonceFromAuthURL(t, s2.Header.Get("Location"))

		resp := h.callback(c, firstState, "code")
		if got := authError(t, resp); got != CodeInvalidFlow {
			t.Fatalf("auth_error = %q, want %q", got, CodeInvalidFlow)
		}
		h.assertNoSessions()
	})
}

// --- accounts ---------------------------------------------------------------

func TestUnverifiedEmailIsRefused(t *testing.T) {
	h := newHarness(t)

	resp := h.signIn(h.client(), func(string) { h.provider.emailVerified = false })
	if got := authError(t, resp); got != CodeUnverifiedEmail {
		t.Fatalf("auth_error = %q, want %q", got, CodeUnverifiedEmail)
	}
	if n := h.countUsers(); n != 0 {
		t.Errorf("users = %d, want 0 -- no user, identity or session may be created", n)
	}
	h.assertNoSessions()
}

// Task 8.5: provisioning against an address another account already holds is
// refused with a code, and nothing is merged.
func TestEmailHeldByAnotherAccountIsRefused(t *testing.T) {
	h := newHarness(t)
	h.seedUser("taken@example.test")

	resp := h.signIn(h.client(), func(string) { h.provider.email = "taken@example.test" })
	if got := authError(t, resp); got != CodeEmailTaken {
		t.Fatalf("auth_error = %q, want %q", got, CodeEmailTaken)
	}
	if n := h.countUsers(); n != 1 {
		t.Errorf("users = %d, want 1 -- the seeded account only, with nothing merged", n)
	}
	h.assertNoIdentities()
}

// Task 8.5b, and the sharpest consequence of design D3a: a returning person
// whose provider address now belongs to somebody else must still sign in. The
// returning path writes nothing to users, so there is no constraint to violate.
func TestReturningSignInWithChangedEmailWritesNothing(t *testing.T) {
	h := newHarness(t)

	first := h.signIn(h.client(), nil)
	first.Body.Close()
	if got := authError(t, first); got != "" {
		t.Fatalf("first sign-in failed with %q", got)
	}
	original := h.emailOfOnlyUser()

	// Somebody else now holds the address this person's provider will report.
	h.seedUser("moved@example.test")

	second := h.signIn(h.client(), func(string) { h.provider.email = "moved@example.test" })
	defer second.Body.Close()
	if got := authError(t, second); got != "" {
		t.Fatalf("returning sign-in refused with %q; a valid account must not be locked out "+
			"by another account's address", got)
	}
	if now := h.emailOf(original); now != original {
		t.Errorf("stored address changed to %q, want %q left as it was", now, original)
	}
}

func TestLocaleClaimIsNormalised(t *testing.T) {
	for _, tc := range []struct{ claim, want string }{
		{"en-GB", "en"},
		{"ru-RU", "ru"},
		{"nb-NO", "en"}, // unsupported: fall back, never refuse
		{"", "en"},
	} {
		t.Run(tc.claim, func(t *testing.T) {
			if got := normaliseLocale(tc.claim); got != tc.want {
				t.Errorf("normaliseLocale(%q) = %q, want %q", tc.claim, got, tc.want)
			}
		})
	}
}

// --- sessions ---------------------------------------------------------------

// Task 8.6: no cookie, an unknown cookie, an expired session and a revoked one
// all reach no authenticated route, and all are indistinguishable.
func TestAnonymousRequestsReachNoAuthenticatedRoute(t *testing.T) {
	t.Run("no cookie", func(t *testing.T) {
		h := newHarness(t)
		if status, body := h.whoami(h.client()); status != http.StatusUnauthorized {
			t.Fatalf("status %d body %q, want 401", status, body)
		}
	})

	t.Run("unknown cookie", func(t *testing.T) {
		h := newHarness(t)
		c := h.client()
		h.setSessionCookie(c, "a-token-nobody-issued")
		if status, _ := h.whoami(c); status != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", status)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		h := newHarness(t)
		h.svc.cfg.SessionLifetime = -time.Minute // born expired

		c := h.client()
		resp := h.signIn(c, nil)
		resp.Body.Close()

		if status, _ := h.whoami(c); status != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401 for a session past its expiry", status)
		}
	})

	t.Run("revoked session", func(t *testing.T) {
		h := newHarness(t)
		c := h.client()

		resp := h.signIn(c, nil)
		resp.Body.Close()
		if status, _ := h.whoami(c); status != http.StatusOK {
			t.Fatalf("precondition: signed in but /whoami says %d", status)
		}

		out, err := c.Post(h.server.URL+"/auth/logout", "", nil)
		if err != nil {
			t.Fatalf("logout: %v", err)
		}
		out.Body.Close()
		if out.StatusCode != http.StatusNoContent {
			t.Fatalf("logout status %d, want 204", out.StatusCode)
		}

		// Present the same cookie again: the server revoked it, so it grants
		// nothing even though the browser still holds it.
		h.setSessionCookie(c, h.lastSessionToken(c))
		if status, _ := h.whoami(c); status != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401 after sign-out", status)
		}
		h.assertSessionRevokedNotDeleted()
	})
}

// Task 8.7: the raw token exists only in the cookie.
func TestRawSessionTokenIsNeverStored(t *testing.T) {
	h := newHarness(t)
	c := h.client()

	resp := h.signIn(c, nil)
	resp.Body.Close()

	token := h.lastSessionToken(c)
	if token == "" {
		t.Fatal("no session cookie was set")
	}

	var stored []byte
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT token_sha256 FROM sessions").Scan(&stored)
	})
	if err != nil {
		t.Fatalf("reading the session row: %v", err)
	}

	want := sha256.Sum256([]byte(token))
	if !bytes.Equal(stored, want[:]) {
		t.Error("the stored value is not the SHA-256 of the cookie value")
	}
	if bytes.Contains(stored, []byte(token)) {
		t.Error("the raw token appears in the database")
	}

	// And nothing else in the row carries it either.
	var rowText string
	err = h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT sessions::text FROM sessions").Scan(&rowText)
	})
	if err != nil {
		t.Fatalf("reading the row as text: %v", err)
	}
	if bytes.Contains([]byte(rowText), []byte(token)) {
		t.Errorf("the raw token appears somewhere in the session row: %s", rowText)
	}
}
