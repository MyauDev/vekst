// Package identity answers "who is calling", and keeps that answer between
// requests.
//
// # These four tables are outside row-level security
//
// users, user_identities, sessions and auth_flows are listed in
// deploy/db/rls-exempt-tables.txt, each for a reason recorded there: none of
// them belongs to an organisation. users is global by design
// (ARCHITECTURE.md 5.5) and access to a tenant's data is mediated by
// memberships, which change 1.1 adds; a session belongs to a person rather
// than an organisation, because tenant context is chosen per request after
// authentication; and an auth_flow exists before anyone is authenticated at
// all, so it cannot have an owner.
//
// Nothing at the database level therefore filters what a query on these
// tables returns. The exposure that creates is cross-person rather than
// cross-tenant -- there is no org_id here to forget -- and it is real: an
// unfiltered read of users returns every user of every customer, which in a
// product whose customers are named companies discloses who those companies
// are and who their finance staff are, without a single figure moving.
//
// Three things hold the boundary instead of a policy:
//
//  1. vekst_app holds no UPDATE privilege on user_identities (migration
//     00003), so the binding between a provider subject and an account cannot
//     be repointed at another person -- an account takeover that no policy
//     would have caught, because it is a legitimate-looking write.
//  2. scripts/check-identity-queries.sh fails the build if any query file
//     other than core/internal/db/query/identity.sql names these tables, or
//     if that file reaches outside them.
//  3. This comment, which now records the reasoning rather than carrying the
//     whole weight of it.
//
// # Reading users once organisations exist
//
// From change 1.1, the way to list the people in an organisation is two
// steps: read memberships under row-level security to get the user ids, then
// fetch those users by key here. Do not add a join -- not because the join is
// unsafe (joining memberships is in fact safe, since its policy re-imposes
// tenancy) but because the narrowness check draws its line at the package
// boundary, and one exception makes the line unenforceable. The query neither
// the check nor a policy can catch is a read of users with no membership
// predicate at all; that is the one to review for.
//
// # Transactions
//
// Everything here runs through db.InSystemTx, which sets no tenant context.
// That is correct and not a shortcut: the middleware resolves a session
// cookie before any organisation is known, so there is no tenant to set. This
// package is one of the small, listed set of legitimate InSystemTx callers,
// and change 1.1 counts those call sites against a committed number.
package identity

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/MyauDev/vekst/core/internal/db"
)

// GoogleIssuer is the issuer whose discovery document describes Google's
// endpoints and JWKS. It is also the `iss` every ID token must carry.
const GoogleIssuer = "https://accounts.google.com"

// Provider names an issuer instance, not a protocol. For Google the two
// collapse, because iss is always GoogleIssuer; they do not collapse for a
// multi-tenant Entra or for SAML, where a NameID is unique only within its
// issuing IdP. A second provider therefore arrives either as an issuer-scoped
// value or by adding an issuer column to the key -- see design D3.
const ProviderGoogle = "google"

// Config is the subset of core's configuration this package needs. Kept
// separate from core/internal/config's exported type so the dependency runs
// one way.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	// SessionLifetime is how long a session grants access after sign-in.
	SessionLifetime time.Duration

	// SessionRetention is how much longer an expired row survives before the
	// expiry job removes it. It governs auditability, never access.
	SessionRetention time.Duration

	// AuthFlowLifetime bounds how long a started sign-in may stay pending.
	AuthFlowLifetime time.Duration

	// CookieSecure sets the Secure attribute on both cookies this package
	// issues.
	CookieSecure bool

	// Issuer overrides the OIDC issuer whose discovery document is read at
	// startup. Empty means GoogleIssuer, which is the only value production
	// ever uses; a test points it at a stub provider so the full redirect
	// chain can be driven with tokens whose signature, audience, expiry and
	// nonce it controls.
	Issuer string
}

// Service serves the three auth routes and resolves sessions for the
// middleware. A nil *Service is the "sign-in is not configured" state: the
// routes still exist and answer with a configuration code, so a developer
// with no OAuth client gets a working stack rather than a nil dereference
// (design D6a).
type Service struct {
	cfg      Config
	database *db.DB
	log      *slog.Logger

	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config

	// now is time.Now except in tests. Nothing else in this package reads the
	// clock, so a test can drive expiry without sleeping.
	now func() time.Time
}

// New discovers the provider's endpoints and JWKS from its well-known
// document and returns a ready Service. It performs I/O and is called once,
// at startup, so that a browser redirect never waits on discovery.
//
// The context bounds discovery: an unreachable provider must fail loudly at
// startup rather than hang the first sign-in.
func New(ctx context.Context, cfg Config, database *db.DB, log *slog.Logger) (*Service, error) {
	issuer := cfg.Issuer
	if issuer == "" {
		issuer = GoogleIssuer
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("identity: discovering %s: %w", issuer, err)
	}

	return &Service{
		cfg:      cfg,
		database: database,
		log:      log,
		// The verifier checks the signature against the provider's JWKS, the
		// issuer, the audience and the expiry. The nonce is checked
		// separately, against the flow row, because only the flow knows it.
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		now: time.Now,
	}, nil
}

// Routes mounts the three auth routes. They are plain HTTP rather than
// Connect because an OIDC redirect flow cannot be a remote procedure call: a
// Connect handler cannot answer with a 302, and the callback arrives as a
// browser navigation carrying query parameters. This is the single exception
// to "the browser talks to core over Connect", and it is an exception about
// transport rather than contract -- no application data crosses these routes
// (design D1).
func (s *Service) Routes(mux chiRouter) {
	mux.Get("/auth/google/start", s.handleStart)
	mux.Get("/auth/google/callback", s.handleCallback)
	mux.Post("/auth/logout", s.handleLogout)
}

// chiRouter is the little of chi.Router this package uses, named so the
// server package can pass its router without this package importing chi's
// full surface.
type chiRouter interface {
	Get(pattern string, h http.HandlerFunc)
	Post(pattern string, h http.HandlerFunc)
}

// configured reports whether sign-in can work. A nil Service is the
// unconfigured state.
func (s *Service) configured() bool {
	return s != nil && s.cfg.ClientID != "" && s.cfg.ClientSecret != "" && s.cfg.RedirectURL != ""
}
