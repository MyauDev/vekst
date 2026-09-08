package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// Error codes. The backend returns codes, never sentences: translation is the
// client's, per ARCHITECTURE.md. They reach the browser as a query parameter
// on the sign-in screen, which is the only place a redirect flow can put them.
const (
	CodeNotConfigured   = "auth_not_configured"
	CodeInvalidFlow     = "invalid_flow"
	CodeInvalidToken    = "invalid_token"
	CodeUnverifiedEmail = "unverified_email"
	CodeEmailTaken      = "email_taken"
	CodeInternal        = "internal_error"
)

// supportedLocales mirrors the CHECK constraint on users.locale.
var supportedLocales = map[string]bool{"en": true, "ru": true}

// postSignInPath is where a completed sign-in lands, and signInPath is where a
// failed one lands. Both are fixed paths, never a caller-supplied return URL:
// that parameter is how a redirect flow becomes an open redirect.
//
// They are two paths because the browser has two surfaces. "/" is the public
// landing page and reads no session at all -- deliberately, so it stays
// separable into a static bundle -- so a completed sign-in returning there
// would show a signed-in person a page inviting them to sign in, which is
// exactly what it did until 2026-09-08. "/app" is the authenticated surface and
// resolves the session itself.
//
// A failure goes to the sign-in screen rather than the landing page, because
// that is the only screen that renders the auth_error code as a sentence. Sent
// to "/" the code would be in the URL and invisible on the page.
const (
	postSignInPath = "/app"
	signInPath     = "/signin"
)

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("identity: generating random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// fail sends the browser back to the sign-in screen carrying a machine-readable
// code. 303 rather than 302 so the browser issues a GET regardless of the
// method that failed.
func (s *Service) fail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, signInPath+"?auth_error="+url.QueryEscape(code), http.StatusSeeOther)
}

// handleStart creates the pending flow and sends the browser to Google.
func (s *Service) handleStart(w http.ResponseWriter, r *http.Request) {
	if !s.configured() {
		s.fail(w, r, CodeNotConfigured)
		return
	}

	state, err := randomToken()
	if err != nil {
		s.log.Error("auth: generating state", "err", err)
		s.fail(w, r, CodeInternal)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		s.log.Error("auth: generating nonce", "err", err)
		s.fail(w, r, CodeInternal)
		return
	}
	// PKCE: the challenge goes to Google now, the verifier is presented at the
	// exchange. It proves whoever redeems the code is who started the flow.
	verifier := oauth2.GenerateVerifier()

	expiresAt := s.now().Add(s.cfg.AuthFlowLifetime)
	var flowID pgtype.UUID
	err = s.database.InSystemTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		flowID, err = gendb.New(tx).InsertAuthFlow(ctx, gendb.InsertAuthFlowParams{
			State:        state,
			Nonce:        nonce,
			CodeVerifier: verifier,
			ExpiresAt:    pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		return err
	})
	if err != nil {
		s.log.Error("auth: creating flow", "err", err)
		s.fail(w, r, CodeInternal)
		return
	}

	// The cookie carries the flow id; state travels in the URL. The callback
	// checks both, so a leaked callback URL is only half of what is needed.
	s.setCookie(w, flowCookie, uuid.UUID(flowID.Bytes).String(), expiresAt)

	http.Redirect(w, r, s.oauth.AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	), http.StatusFound)
}

// handleCallback completes the flow: both halves checked, code exchanged, token
// verified, account resolved, session issued.
func (s *Service) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !s.configured() {
		s.fail(w, r, CodeNotConfigured)
		return
	}

	state := r.URL.Query().Get("state")
	cookie, cookieErr := r.Cookie(flowCookie)
	// The cookie is checked for presence before the flow is consumed, so a
	// caller who somehow knows a state value cannot burn a pending sign-in
	// without also holding the browser's cookie.
	if state == "" || cookieErr != nil || cookie.Value == "" {
		s.fail(w, r, CodeInvalidFlow)
		return
	}

	var flow gendb.ConsumeAuthFlowRow
	err := s.database.InSystemTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		flow, err = gendb.New(tx).ConsumeAuthFlow(ctx, state)
		return err
	})
	s.clearCookie(w, flowCookie)
	if err != nil {
		// No row: an unknown state, an expired flow, or one already consumed.
		// All three are refused identically -- the caller learns nothing from
		// the difference, and a replayed callback cannot succeed because the
		// DELETE that consumed it already returned its only row.
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("auth: consuming flow", "err", err)
		}
		s.fail(w, r, CodeInvalidFlow)
		return
	}

	if uuid.UUID(flow.ID.Bytes).String() != cookie.Value {
		// The state matched a live flow, but this browser did not start it.
		s.fail(w, r, CodeInvalidFlow)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		s.fail(w, r, CodeInvalidFlow)
		return
	}

	token, err := s.oauth.Exchange(r.Context(), code, oauth2.VerifierOption(flow.CodeVerifier))
	if err != nil {
		s.log.Warn("auth: code exchange failed", "err", err)
		s.fail(w, r, CodeInvalidToken)
		return
	}

	claims, err := s.verifyIDToken(r.Context(), token, flow.Nonce)
	if err != nil {
		s.log.Warn("auth: id token rejected", "err", err)
		s.fail(w, r, CodeInvalidToken)
		return
	}
	if !claims.EmailVerified {
		s.fail(w, r, CodeUnverifiedEmail)
		return
	}

	userID, code2 := s.resolveOrProvision(r.Context(), claims)
	if code2 != "" {
		s.fail(w, r, code2)
		return
	}

	if err := s.createSession(r.Context(), w, r, userID); err != nil {
		s.log.Error("auth: creating session", "err", err)
		s.fail(w, r, CodeInternal)
		return
	}

	http.Redirect(w, r, postSignInPath, http.StatusFound)
}

// handleLogout revokes the session and clears the cookie in one response.
func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" && s != nil {
		if err := s.revoke(r.Context(), c.Value); err != nil {
			s.log.Error("auth: revoking session", "err", err)
		}
	}
	if s != nil {
		s.clearCookie(w, sessionCookie)
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveOrProvision implements design D3a: a known subject uses its account
// and writes nothing; an unknown one provisions. It returns an error code
// rather than an error, because every failure here is one the browser is told
// about by name.
func (s *Service) resolveOrProvision(ctx context.Context, claims *idClaims) (pgtype.UUID, string) {
	var userID pgtype.UUID

	err := s.database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		ident, err := q.FindIdentity(ctx, gendb.FindIdentityParams{
			Provider: ProviderGoogle,
			Subject:  claims.Subject,
		})
		if err == nil {
			// Known subject. The account record is provisioned once and never
			// written by a later claim, so a provider email that has changed
			// since last time simply goes stale -- and cannot collide with
			// another account, because nothing is written (design D3a).
			userID = ident.UserID
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		u, err := q.InsertUser(ctx, gendb.InsertUserParams{
			Email:  text(strings.ToLower(claims.Email)),
			Name:   text(claims.Name),
			Locale: normaliseLocale(claims.Locale),
		})
		if err != nil {
			return err
		}
		if err := q.InsertIdentity(ctx, gendb.InsertIdentityParams{
			Provider: ProviderGoogle,
			Subject:  claims.Subject,
			UserID:   u.ID,
		}); err != nil {
			return err
		}
		userID = u.ID
		return nil
	})

	switch {
	case err == nil:
		return userID, ""
	case isUniqueViolation(err, "users_email_key"):
		// The address belongs to an account this subject does not own. Refused
		// with a code, never merged: merging accounts is a deliberate operation
		// with a UI, and it is not in this change.
		return pgtype.UUID{}, CodeEmailTaken
	default:
		s.log.Error("auth: resolving account", "err", err)
		return pgtype.UUID{}, CodeInternal
	}
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// normaliseLocale maps a BCP-47 tag onto the supported set. Google reports
// tags like "en-GB" and "nb-NO"; the raw claim would fail users.locale's CHECK
// on an ordinary sign-in, and an unsupported language is not a reason to
// refuse someone.
func normaliseLocale(claim string) string {
	base, _, _ := strings.Cut(strings.ToLower(claim), "-")
	if supportedLocales[base] {
		return base
	}
	return "en"
}
