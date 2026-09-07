package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

const (
	// sessionCookie carries the opaque session token. Only its SHA-256 is
	// stored, so a read of the sessions table yields nothing presentable.
	sessionCookie = "vekst_session"

	// flowCookie carries the auth_flows row id for an in-flight sign-in. It
	// holds the id rather than the state value, which travels in the URL: two
	// halves in two channels means a leaked callback URL -- referrer, history,
	// a shoulder-surfed address bar -- carries only one of them (design D1).
	flowCookie = "vekst_auth_flow"
)

// ErrNoSession is returned when a request carries no usable session. It is
// deliberately the same error for "no cookie", "unknown cookie", "expired"
// and "revoked": the caller learns nothing from the difference, and the
// middleware answers all four identically.
var ErrNoSession = errors.New("identity: no live session")

// User is the authenticated person, as this package hands them to the rest of
// core. It carries no organisation and no role -- there is no organisation
// model until change 1.1, and a field that exists before it means anything is
// a field the front end starts trusting.
type User struct {
	ID     uuid.UUID
	Email  string
	Name   string
	Locale string
}

// newSessionToken returns 32 bytes of crypto/rand, base64url-encoded. The
// caller puts this in a cookie and stores only its hash.
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("identity: generating session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken is the one-way step between the cookie and the database. Lookup
// by hash is a unique-index probe, so storing the hash costs nothing in speed
// and means a backup, a log line or a query gone wrong yields no credential.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// createSession issues a token, stores its hash, and sets the cookie. It runs
// only after a successful token exchange: no session exists before that, which
// is what makes session fixation impossible here.
func (s *Service) createSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID pgtype.UUID) error {
	token, err := newSessionToken()
	if err != nil {
		return err
	}

	expiresAt := s.now().Add(s.cfg.SessionLifetime)
	err = s.database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertSession(ctx, gendb.InsertSessionParams{
			TokenSha256: hashToken(token),
			UserID:      userID,
			ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
			UserAgent:   text(r.UserAgent()),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("identity: creating session: %w", err)
	}

	s.setCookie(w, sessionCookie, token, expiresAt)
	return nil
}

// Resolve turns a request's session cookie into the person it belongs to.
// Expiry and revocation are predicates in the query rather than checks here,
// so a revoked or expired session simply matches nothing.
func (s *Service) Resolve(ctx context.Context, r *http.Request) (User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return User{}, ErrNoSession
	}

	var row gendb.FindLiveSessionWithUserRow
	err = s.database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		var err error
		row, err = q.FindLiveSessionWithUser(ctx, hashToken(c.Value))
		if err != nil {
			return err
		}
		// Touching last_seen_at inside the same transaction keeps the read and
		// the write on one round trip. It is the only write a plain
		// authenticated request makes.
		return q.TouchSession(ctx, row.SessionID)
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return User{}, ErrNoSession
	case err != nil:
		return User{}, fmt.Errorf("identity: resolving session: %w", err)
	}

	return User{
		ID:     uuid.UUID(row.UserID.Bytes),
		Email:  row.UserEmail.String,
		Name:   row.UserName.String,
		Locale: row.UserLocale,
	}, nil
}

// revoke marks the session behind a cookie revoked. Revocation rather than
// deletion is the point of a server-side session: a signed stateless token
// cannot offer it, and the row stays auditable until the expiry job removes
// it.
func (s *Service) revoke(ctx context.Context, token string) error {
	return s.database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return gendb.New(tx).RevokeSession(ctx, hashToken(token))
	})
}

// setCookie issues one of this package's two cookies.
//
// SameSite=Lax rather than Strict, for a different reason per cookie. The flow
// cookie must survive the callback, which is a top-level GET navigation from
// Google -- Strict would withhold it on arrival and every sign-in would fail.
// The session cookie is never sent on the callback, so that argument does not
// apply to it; it wants Lax so a person following an external link into the
// app is not presented as signed out.
//
// No Domain attribute: the cookie stays host-only, which is what keeps it
// first-party to the single origin serving the document, /rpc and /auth.
func (s *Service) setCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// text maps Go's empty string to SQL NULL. users.email is nullable because
// not every provider asserts one, and Postgres treats NULLs as distinct in a
// unique index, so any number of address-less accounts coexist.
func text(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}
