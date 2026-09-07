package identity

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

const testClientID = "stub-client-id"

// harness is a running core with sign-in wired to a stub provider, plus the
// live database the identity tables need. Sessions, flows and accounts are
// real rows throughout: none of this is worth testing against a mock, because
// every property under test is a property of the schema or of a real verifier.
type harness struct {
	svc      *Service
	provider *stubProvider
	server   *httptest.Server
	database *db.DB
	t        *testing.T
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	database, err := db.New(context.Background(), db.Config{URL: dsn})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(database.Close)

	provider := newStubProvider(t, testClientID)

	h := &harness{provider: provider, database: database, t: t}
	h.cleanTables()

	svc, err := New(context.Background(), Config{
		ClientID:         testClientID,
		ClientSecret:     "stub-client-secret",
		SessionLifetime:  time.Hour,
		SessionRetention: time.Hour,
		AuthFlowLifetime: 10 * time.Minute,
		CookieSecure:     false, // httptest serves plain HTTP
		Issuer:           provider.issuer(),
	}, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("identity.New against the stub provider: %v", err)
	}
	h.svc = svc

	r := chi.NewRouter()
	r.Use(svc.Middleware)
	svc.Routes(r)
	// A stand-in for an authenticated route: it reports who the middleware
	// resolved, so a test can assert what a request is allowed to be.
	r.Get("/whoami", func(w http.ResponseWriter, req *http.Request) {
		u, ok := FromContext(req.Context())
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, CodeUnauthenticated)
			return
		}
		_, _ = io.WriteString(w, u.ID.String())
	})

	h.server = httptest.NewServer(r)
	t.Cleanup(h.server.Close)

	// The redirect URL must be the running test server's, which is only known
	// now. Rebuilding the oauth config is cheaper than deferring construction.
	svc.cfg.RedirectURL = h.server.URL + "/auth/google/callback"
	svc.oauth.RedirectURL = svc.cfg.RedirectURL

	return h
}

// cleanTables gives each test an empty slate. These four tables are global, so
// tests would otherwise collide through them.
func (h *harness) cleanTables() {
	h.t.Helper()
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		for _, stmt := range []string{
			"DELETE FROM sessions",
			"DELETE FROM auth_flows",
			"DELETE FROM user_identities",
			"DELETE FROM users",
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.t.Fatalf("clearing identity tables: %v", err)
	}
}

// client returns an HTTP client that keeps cookies but does not follow
// redirects, so a test can inspect every hop of the chain itself.
func (h *harness) client() *http.Client {
	h.t.Helper()
	jar, err := newJar()
	if err != nil {
		h.t.Fatalf("building cookie jar: %v", err)
	}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// signIn drives the whole redirect chain: start, the provider's authorization
// step, and the callback. It returns the callback's response so a test can
// assert on where it sent the browser.
//
// tweak runs after the authorization URL is known and before the callback, so
// a test can point the stub's knobs at a bad signature, a wrong audience, an
// expired token or a mismatched nonce.
func (h *harness) signIn(c *http.Client, tweak func(authURL string)) *http.Response {
	h.t.Helper()

	start, err := c.Get(h.server.URL + "/auth/google/start")
	if err != nil {
		h.t.Fatalf("GET /auth/google/start: %v", err)
	}
	defer start.Body.Close()
	if start.StatusCode != http.StatusFound {
		h.t.Fatalf("start: status %d, want 302", start.StatusCode)
	}
	authURL := start.Header.Get("Location")
	assertPKCEChallenge(h.t, authURL)

	// By default the provider mints a token matching this flow. A tweak may
	// change that.
	h.provider.nonce = nonceFromAuthURL(h.t, authURL)
	if tweak != nil {
		tweak(authURL)
	}

	state := stateFromAuthURL(h.t, authURL)
	return h.callback(c, state, "stub-auth-code")
}

func (h *harness) callback(c *http.Client, state, code string) *http.Response {
	h.t.Helper()
	u := h.server.URL + "/auth/google/callback?" + url.Values{
		"state": {state}, "code": {code},
	}.Encode()
	resp, err := c.Get(u)
	if err != nil {
		h.t.Fatalf("GET callback: %v", err)
	}
	return resp
}

// authError reads the error code a failed auth route redirected with. Empty
// means the response was a success redirect.
func authError(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parsing redirect %q: %v", loc, err)
	}
	return u.Query().Get("auth_error")
}

// whoami returns the body of the authenticated stand-in route.
func (h *harness) whoami(c *http.Client) (int, string) {
	h.t.Helper()
	resp, err := c.Get(h.server.URL + "/whoami")
	if err != nil {
		h.t.Fatalf("GET /whoami: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func (h *harness) countUsers() int {
	h.t.Helper()
	var n int
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n)
	})
	if err != nil {
		h.t.Fatalf("counting users: %v", err)
	}
	return n
}

// seedUser inserts an account directly, for the cases that need one to already
// exist -- an address another person holds, say.
func (h *harness) seedUser(email string) {
	h.t.Helper()
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertUser(ctx, gendb.InsertUserParams{
			Email:  text(email),
			Locale: "en",
		})
		return err
	})
	if err != nil {
		h.t.Fatalf("seeding user %s: %v", email, err)
	}
}

// --- assertions on the tables ----------------------------------------------

func (h *harness) scalar(query string) int {
	h.t.Helper()
	var n int
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, query).Scan(&n)
	})
	if err != nil {
		h.t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func (h *harness) assertNoSessions() {
	h.t.Helper()
	if n := h.scalar("SELECT count(*) FROM sessions"); n != 0 {
		h.t.Errorf("sessions = %d, want 0 -- a session may exist only after a successful exchange", n)
	}
}

func (h *harness) assertNoIdentities() {
	h.t.Helper()
	if n := h.scalar("SELECT count(*) FROM user_identities"); n != 0 {
		h.t.Errorf("user_identities = %d, want 0", n)
	}
}

// assertSessionRevokedNotDeleted holds the spec to its word: signing out marks
// the row revoked rather than deleting it silently, so it stays auditable until
// the expiry job removes it.
func (h *harness) assertSessionRevokedNotDeleted() {
	h.t.Helper()
	if n := h.scalar("SELECT count(*) FROM sessions"); n != 1 {
		h.t.Fatalf("sessions = %d after sign-out, want the row to still exist", n)
	}
	if n := h.scalar("SELECT count(*) FROM sessions WHERE revoked_at IS NOT NULL"); n != 1 {
		h.t.Error("the session row exists but is not marked revoked")
	}
}

func (h *harness) emailOfOnlyUser() string {
	h.t.Helper()
	var email string
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT u.email FROM users u JOIN user_identities i ON i.user_id = u.id`).Scan(&email)
	})
	if err != nil {
		h.t.Fatalf("reading the signed-in user's address: %v", err)
	}
	return email
}

// emailOf returns the address of the account that holds it, or "" -- used to
// assert an address did not move.
func (h *harness) emailOf(email string) string {
	h.t.Helper()
	var got string
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(max(u.email), '') FROM users u
			 JOIN user_identities i ON i.user_id = u.id
			 WHERE u.email = $1`, email).Scan(&got)
	})
	if err != nil {
		h.t.Fatalf("reading address %q: %v", email, err)
	}
	return got
}

// --- cookies ----------------------------------------------------------------

func (h *harness) serverURL() *url.URL {
	h.t.Helper()
	u, err := url.Parse(h.server.URL)
	if err != nil {
		h.t.Fatalf("parsing server URL: %v", err)
	}
	return u
}

func (h *harness) lastSessionToken(c *http.Client) string {
	h.t.Helper()
	for _, ck := range c.Jar.Cookies(h.serverURL()) {
		if ck.Name == sessionCookie {
			return ck.Value
		}
	}
	return ""
}

func (h *harness) setSessionCookie(c *http.Client, value string) {
	h.t.Helper()
	c.Jar.SetCookies(h.serverURL(), []*http.Cookie{{
		Name: sessionCookie, Value: value, Path: "/",
	}})
}
