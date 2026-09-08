package identity

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Task 7.2: expired rows go, live rows stay.
//
// The distinction that matters is between *expiry* and *retention*. A session
// stops granting access at expires_at, enforced by a predicate in the query --
// this job never gates access. It only decides when a dead row stops being
// readable, which is why a session that expired a minute ago must survive and
// one that expired longer ago than the retention window must not.
func TestExpiryJobRemovesDeadRowsAndKeepsLiveOnes(t *testing.T) {
	h := newHarness(t)

	// SessionRetention is an hour in the harness, so the cutoff is one hour
	// ago.
	seed := []string{
		`INSERT INTO auth_flows (state, nonce, code_verifier, expires_at)
		 VALUES ('flow-expired', 'n', 'v', now() - interval '1 minute')`,
		`INSERT INTO auth_flows (state, nonce, code_verifier, expires_at)
		 VALUES ('flow-live', 'n', 'v', now() + interval '10 minutes')`,
		`INSERT INTO users (id, email) VALUES
		 ('11111111-1111-1111-1111-111111111111', 'expiry@example.test')`,
		// Expired long enough ago to be past retention.
		`INSERT INTO sessions (token_sha256, user_id, expires_at)
		 VALUES ('\x01', '11111111-1111-1111-1111-111111111111', now() - interval '2 hours')`,
		// Expired, but still inside the retention window: dead for access,
		// still readable to an operator asking what happened.
		`INSERT INTO sessions (token_sha256, user_id, expires_at)
		 VALUES ('\x02', '11111111-1111-1111-1111-111111111111', now() - interval '1 minute')`,
		// Live.
		`INSERT INTO sessions (token_sha256, user_id, expires_at)
		 VALUES ('\x03', '11111111-1111-1111-1111-111111111111', now() + interval '1 hour')`,
	}
	err := h.database.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		for _, stmt := range seed {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := h.svc.expire(context.Background()); err != nil {
		t.Fatalf("expire: %v", err)
	}

	if n := h.scalar("SELECT count(*) FROM auth_flows"); n != 1 {
		t.Errorf("auth_flows = %d, want 1 (the live one)", n)
	}
	if n := h.scalar("SELECT count(*) FROM auth_flows WHERE state = 'flow-live'"); n != 1 {
		t.Error("the flow still inside its lifetime was removed")
	}

	if n := h.scalar("SELECT count(*) FROM sessions"); n != 2 {
		t.Errorf("sessions = %d, want 2 (the live one and the recently expired one)", n)
	}
	if n := h.scalar(`SELECT count(*) FROM sessions WHERE token_sha256 = '\x01'`); n != 0 {
		t.Error("a session past its retention window was kept")
	}
	if n := h.scalar(`SELECT count(*) FROM sessions WHERE token_sha256 = '\x03'`); n != 1 {
		t.Error("a live session was removed")
	}
}

// The job takes no organisation, and that is deliberate rather than an
// omission: these four tables carry no tenant data. Every worker from change
// 1.1 onward must take its tenant identifier from its own arguments instead.
func TestExpiryJobKindIsStable(t *testing.T) {
	if got := (ExpiryArgs{}).Kind(); got != "identity_expiry" {
		t.Errorf("Kind() = %q; River identifies persisted jobs by this string", got)
	}
}
