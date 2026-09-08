package migrate

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"testing"
)

// appURL rewrites a vekst_migrator connection string to authenticate as
// vekst_app instead, so a test can exercise what the application role can and
// cannot do rather than what the schema owner can.
func appURL(t *testing.T, migratorURL string) string {
	t.Helper()
	u, err := url.Parse(migratorURL)
	if err != nil {
		t.Fatalf("parsing migrator URL: %v", err)
	}
	u.User = url.UserPassword("vekst_app", "vekst_app")
	return u.String()
}

// The identity binding is insert-once, and 00003 revokes UPDATE on
// user_identities from vekst_app so that stays true no matter what a query
// file later says. One `ON CONFLICT (provider, subject) DO UPDATE SET user_id`
// would repoint a Google account at a different person, which after change 1.1
// hands the holder every organisation that person belongs to -- and no
// row-level-security policy would catch it, because it is a legitimate-looking
// write by the application role. See add-identity design D3b.
func TestIdentityBindingCannotBeRepointed(t *testing.T) {
	migratorURL := testMigratorURL(t, "vekst_migrate_privileges_test")
	if err := Up(context.Background(), migratorURL); err != nil {
		t.Fatalf("Up: %v", err)
	}

	migratorDB, err := sql.Open("pgx", migratorURL)
	if err != nil {
		t.Fatalf("opening migrator connection: %v", err)
	}
	defer migratorDB.Close()

	// The privilege grid: UPDATE is absent on user_identities and present on
	// the other three. Asserting the neighbours too is what proves the revoke
	// was aimed rather than a blanket one that happened to land here.
	for _, tc := range []struct {
		table      string
		wantUpdate bool
	}{
		{"user_identities", false},
		{"users", true},
		{"sessions", true},
		{"auth_flows", true},
	} {
		var has bool
		if err := migratorDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.table_privileges
				WHERE grantee = 'vekst_app'
				  AND table_name = $1
				  AND privilege_type = 'UPDATE'
			)`, tc.table).Scan(&has); err != nil {
			t.Fatalf("reading privileges for %s: %v", tc.table, err)
		}
		if has != tc.wantUpdate {
			t.Errorf("vekst_app UPDATE on %s = %v, want %v", tc.table, has, tc.wantUpdate)
		}
	}

	// vekst_app must still be able to read and insert, or sign-in cannot work
	// at all -- a revoke that took too much would otherwise pass the check
	// above and fail in production.
	for _, priv := range []string{"SELECT", "INSERT"} {
		var has bool
		if err := migratorDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.table_privileges
				WHERE grantee = 'vekst_app'
				  AND table_name = 'user_identities'
				  AND privilege_type = $1
			)`, priv).Scan(&has); err != nil {
			t.Fatalf("reading %s privilege: %v", priv, err)
		}
		if !has {
			t.Errorf("vekst_app lacks %s on user_identities; sign-in cannot work", priv)
		}
	}

	// And the live attempt: two accounts, one binding, and vekst_app trying to
	// move it from one to the other. The catalog says it should fail; this is
	// the proof that it does.
	var victimID, attackerID string
	if err := migratorDB.QueryRow(
		`INSERT INTO users (email) VALUES ('victim@example.test') RETURNING id`).Scan(&victimID); err != nil {
		t.Fatalf("seeding victim: %v", err)
	}
	if err := migratorDB.QueryRow(
		`INSERT INTO users (email) VALUES ('attacker@example.test') RETURNING id`).Scan(&attackerID); err != nil {
		t.Fatalf("seeding attacker: %v", err)
	}
	if _, err := migratorDB.Exec(
		`INSERT INTO user_identities (provider, subject, user_id) VALUES ('google', 'sub-victim', $1)`,
		victimID); err != nil {
		t.Fatalf("seeding binding: %v", err)
	}

	appDB, err := sql.Open("pgx", appURL(t, migratorURL))
	if err != nil {
		t.Fatalf("opening app connection: %v", err)
	}
	defer appDB.Close()

	_, err = appDB.Exec(
		`UPDATE user_identities SET user_id = $1 WHERE provider = 'google' AND subject = 'sub-victim'`,
		attackerID)
	if err == nil {
		t.Fatal("vekst_app repointed an identity at another user; the REVOKE in 00003 is not doing its job")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("repoint failed, but not on privilege: %v", err)
	}

	// The binding still names the victim.
	var owner string
	if err := migratorDB.QueryRow(
		`SELECT user_id FROM user_identities WHERE provider = 'google' AND subject = 'sub-victim'`).Scan(&owner); err != nil {
		t.Fatalf("re-reading binding: %v", err)
	}
	if owner != victimID {
		t.Fatalf("binding moved to %s, want %s", owner, victimID)
	}
}
