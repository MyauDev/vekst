package migrate

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

// Task 5.1/5.2/5.5. The real schema, after every migration, must be clean.
//
// A new tenant table with no policy fails here, on the pull request that adds
// it -- which is the point, and why 0.2 seeded the allowlist before the test
// existed to read it.
func TestRLSCoverageOfTheRealSchema(t *testing.T) {
	url := testMigratorURL(t, "vekst_rls_coverage_test")
	if err := Up(context.Background(), url); err != nil {
		t.Fatalf("Up: %v", err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("opening connection: %v", err)
	}
	defer db.Close()

	findings, err := checkRLSCoverage(db, allowlistPath())
	if err != nil {
		t.Fatalf("running the coverage check: %v", err)
	}
	for _, f := range findings {
		t.Errorf("row-level-security coverage: %s", f)
	}

	// Task 5.5: the allowlist names exactly the tables that exist and are
	// exempt -- no more. A stale line is a table that was dropped and a
	// standing permission to re-create it without a policy.
	exempt, err := exemptTables(allowlistPath())
	if err != nil {
		t.Fatalf("reading the allowlist: %v", err)
	}
	for name := range exempt {
		var exists bool
		if err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
				 WHERE n.nspname = 'public' AND c.relname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("checking %s: %v", name, err)
		}
		if !exists {
			t.Errorf("allowlist names %q, which no longer exists; a stale exemption is "+
				"standing permission to recreate that table with no policy", name)
		}
	}

	// And 00004's four tables are not among them.
	for _, name := range []string{"organizations", "entities", "accounts", "memberships"} {
		if exempt[name] {
			t.Errorf("%s is allowlisted; it is a tenant table and must be covered", name)
		}
	}
}

// Task 5.3. The test must be able to fail -- once per condition it asserts.
//
// Each case creates the offending object in a scratch database, runs the
// checker, asserts it reports that object, and drops it. Without this the
// suite proves only that the current schema happens to pass, and the likeliest
// real failure is a policy that exists and does not isolate, not a table with
// no policy at all.
func TestTheCheckerCanFail(t *testing.T) {
	url := testMigratorURL(t, "vekst_rls_checker_test")
	if err := Up(context.Background(), url); err != nil {
		t.Fatalf("Up: %v", err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("opening connection: %v", err)
	}
	defer db.Close()

	// Confirm the baseline is clean, or every case below would "pass" on
	// somebody else's finding.
	if findings, err := checkRLSCoverage(db, allowlistPath()); err != nil {
		t.Fatalf("baseline check: %v", err)
	} else if len(findings) != 0 {
		t.Fatalf("baseline schema is not clean, so this test cannot attribute failures: %v", findings)
	}

	for _, tc := range []struct {
		name     string
		setup    []string
		teardown []string
		want     string
	}{
		{
			name: "a table with no policy",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL)`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     "probe does not have row-level security enabled",
		},
		{
			name: "a policy that is unconditionally true",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`ALTER TABLE probe FORCE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR ALL USING (true) WITH CHECK (true)`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     `restricts rows with "true"`,
		},
		{
			name: "a policy on the wrong column",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL, created_by uuid NOT NULL)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`ALTER TABLE probe FORCE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR ALL USING (created_by = app_current_org())`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     `restricts rows with "(created_by = app_current_org())"`,
		},
		{
			name: "a policy covering reads only",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`ALTER TABLE probe FORCE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR SELECT USING (org_id = app_current_org())`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     "has no policy covering writes",
		},
		{
			name: "enabled but not forced",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR ALL USING (org_id = app_current_org()) WITH CHECK (org_id = app_current_org())`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     "enabled but not FORCEd",
		},
		{
			name: "a second role-scoped policy",
			setup: []string{
				`CREATE TABLE probe (id uuid PRIMARY KEY, org_id uuid NOT NULL)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`ALTER TABLE probe FORCE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR ALL USING (org_id = app_current_org()) WITH CHECK (org_id = app_current_org())`,
				`CREATE POLICY sneaky ON probe FOR SELECT TO vekst_membership_reader USING (true)`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     "expected exactly one role-scoped policy",
		},
		{
			name: "a second SECURITY DEFINER function",
			setup: []string{
				`CREATE FUNCTION sneaky() RETURNS int LANGUAGE sql SECURITY DEFINER AS $$ SELECT 1 $$`,
				`GRANT EXECUTE ON FUNCTION sneaky() TO vekst_app`,
			},
			teardown: []string{`DROP FUNCTION sneaky()`},
			want:     "expected exactly one SECURITY DEFINER function",
		},
		{
			// The relkind blind spot. A materialized view over a tenant table
			// has no row-level security at all, and 00001's ALTER DEFAULT
			// PRIVILEGES grants vekst_app SELECT on it automatically -- yet it
			// is invisible to information_schema.tables, to pg_tables, and to
			// pg_class filtered on relkind='r'. Verified against Postgres 16:
			// vekst_app read another organisation's rows out of one with no
			// tenant context set at all.
			name: "a materialized view over a tenant table",
			setup: []string{
				`CREATE MATERIALIZED VIEW probe_mv AS SELECT 1 AS org_id`,
			},
			teardown: []string{`DROP MATERIALIZED VIEW probe_mv`},
			want:     "is a materialized view",
		},
		{
			// A partition inherits neither its parent's RLS flags nor its
			// policies, and a direct select on it bypasses the parent's.
			name: "a partition of a tenant table",
			setup: []string{
				`CREATE TABLE probe (org_id uuid NOT NULL, period date NOT NULL) PARTITION BY RANGE (period)`,
				`ALTER TABLE probe ENABLE ROW LEVEL SECURITY`,
				`ALTER TABLE probe FORCE ROW LEVEL SECURITY`,
				`CREATE POLICY p ON probe FOR ALL USING (org_id = app_current_org()) WITH CHECK (org_id = app_current_org())`,
				`CREATE TABLE probe_2026q1 PARTITION OF probe FOR VALUES FROM ('2026-01-01') TO ('2026-04-01')`,
			},
			teardown: []string{`DROP TABLE probe`},
			want:     "is a partition and needs its own policy",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stmt := range tc.setup {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("setting up %q: %v", stmt, err)
				}
			}
			defer func() {
				for _, stmt := range tc.teardown {
					if _, err := db.Exec(stmt); err != nil {
						t.Errorf("tearing down %q: %v", stmt, err)
					}
				}
			}()

			findings, err := checkRLSCoverage(db, allowlistPath())
			if err != nil {
				t.Fatalf("running the coverage check: %v", err)
			}
			if !anyContains(findings, tc.want) {
				t.Fatalf("the checker did not report this condition.\nwant a finding containing: %s\ngot: %v",
					tc.want, findings)
			}
		})
	}

	// The role assertion, which needs its own case because vekst_migrator
	// cannot produce the condition: granting BYPASSRLS requires a superuser,
	// and the migrator deliberately is not one. That is a real part of the
	// defence -- a compromised migrator cannot grant itself the bypass -- so
	// the throwaway superuser the harness uses for CREATE DATABASE sets it
	// here instead.
	//
	// Role attributes are cluster-wide, so this would be unsafe if another
	// package's tests shared this cluster. They do not: core/internal/migrate
	// runs against its own Postgres in CI precisely because its round trip
	// drops vekst_app cluster-wide.
	t.Run("a role that can bypass row-level security", func(t *testing.T) {
		admin, err := sql.Open("pgx", os.Getenv("DATABASE_URL_ADMIN"))
		if err != nil {
			t.Fatalf("opening admin connection: %v", err)
		}
		defer admin.Close()

		if _, err := admin.Exec(`ALTER ROLE vekst_app BYPASSRLS`); err != nil {
			t.Fatalf("granting BYPASSRLS: %v", err)
		}
		defer func() {
			if _, err := admin.Exec(`ALTER ROLE vekst_app NOBYPASSRLS`); err != nil {
				t.Errorf("restoring NOBYPASSRLS: %v", err)
			}
		}()

		findings, err := checkRLSCoverage(db, allowlistPath())
		if err != nil {
			t.Fatalf("running the coverage check: %v", err)
		}
		if !anyContains(findings, "vekst_app has BYPASSRLS") {
			t.Fatalf("the checker did not report BYPASSRLS on vekst_app: %v", findings)
		}
	})

	// After every case has cleaned up, the schema must be clean again -- or a
	// later case passed on an earlier one's leftovers.
	if findings, err := checkRLSCoverage(db, allowlistPath()); err != nil {
		t.Fatalf("final check: %v", err)
	} else if len(findings) != 0 {
		t.Errorf("a case leaked state; schema is not clean at the end: %v", findings)
	}
}

func anyContains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
