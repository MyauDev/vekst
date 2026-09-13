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

	// TLS, not plain HTTP. The cookies this package issues are Secure without
	// exception, and a cookie jar will not send a Secure cookie back over http --
	// so a plaintext harness could only ever test the flow with that attribute
	// switched off, which is the one configuration production never runs.
	h.server = httptest.NewTLSServer(r)
	t.Cleanup(h.server.Close)

	// The redirect URL must be the running test server's, which is only known
	// now. Rebuilding the oauth config is cheaper than deferring construction.
	svc.cfg.RedirectURL = h.server.URL + "/auth/google/callback"
	svc.oauth.RedirectURL = svc.cfg.RedirectURL

	return h
}

// cleanTables gives each test an empty slate. These four tables are global, so
// tests would otherwise collide through them.
//
// The users delete is scoped by email, and has to be. `DELETE FROM users` took
// every user in the database, including ones other packages' tests had just
// created -- a race that was invisible until change 1.1 gave memberships a
// foreign key to users, at which point it stopped silently deleting their rows
// and started failing with 23503 instead. Scoping it is the fix for both: this
// package owns the @example.test domain, and users with no address at all,
// which only sign-in produces (a provider that asserts no email).
//
// It cannot instead delete users that no membership references: memberships is
// a tenant table, so reading it without a tenant context raises -- and this
// runs in InSystemTx, which by definition has none.
func (h *harness) cleanTables() {
	h.t.Helper()

	// Refuse to run against a database holding accounts this harness did not
	// create. DATABASE_URL is whatever the environment says, and pointing it at
	// a development database is one shell variable away -- which happened on
	// 2026-09-08 and locked a real Google account out of the local cluster.
	//
	// The lockout is worth understanding, because the schema is designed to
	// prevent exactly it. An identity is (provider, subject); users.email is
	// descriptive and UNIQUE. Delete a person's user_identities row and leave
	// their users row, and sign-in can no longer find them by subject, so it
	// provisions instead -- and provisioning collides with their own surviving
	// email. CLAUDE.md puts it plainly: the rule exists to keep "a UNIQUE
	// violation off the login path where it would lock out a valid account".
	// Half-deleting puts it straight back on.
	var foreign int
	if err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT count(*) FROM users WHERE email IS NOT NULL AND email NOT LIKE '%@example.test'",
		).Scan(&foreign)
	}); err != nil {
		h.t.Fatalf("checking the database is safe to clear: %v", err)
	}
	if foreign > 0 {
		h.t.Fatalf("DATABASE_URL points at a database holding %d account(s) this harness "+
			"did not create; refusing to clear it. Point DATABASE_URL at a throwaway "+
			"database -- these tests delete every row in sessions, auth_flows and the "+
			"test users' identities.", foreign)
	}

	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		for _, stmt := range []string{
			"DELETE FROM sessions",
			"DELETE FROM auth_flows",
			// Scoped to the same users the next statement removes. Deleting
			// every identity while deleting only test users is what orphans an
			// account: the binding goes, the row stays, and nothing can sign in
			// as it or re-create it.
			"DELETE FROM user_identities WHERE user_id IN " +
				"(SELECT id FROM users WHERE email LIKE '%@example.test' OR email IS NULL)",
			"DELETE FROM users WHERE email LIKE '%@example.test' OR email IS NULL",
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
		Jar: jar,
		// The transport from the test server trusts its self-signed certificate.
		Transport:     h.server.Client().Transport,
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

// countUsers counts the accounts this package owns, scoped the same way
// cleanTables deletes them and for the same reason: users is global and shared
// with every other package's tests against the same database, so an unscoped
// count asserts on rows this package did not create and cannot control.
func (h *harness) countUsers() int {
	h.t.Helper()
	var n int
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT count(*) FROM users WHERE email LIKE '%@example.test' OR email IS NULL").Scan(&n)
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
