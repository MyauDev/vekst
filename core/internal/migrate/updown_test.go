package migrate

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"
)

// testMigratorURL provisions a dedicated, disposable database and returns a
// connection string to it -- never the shared database core/internal/db and
// core/internal/jobs tests also use. Up and Down here drop and recreate the
// whole schema, and `go test ./...` runs different packages' tests
// concurrently by default; sharing a database with those tests would race.
//
// The CREATE and DROP go through DATABASE_URL_ADMIN, a throwaway superuser,
// rather than through vekst_migrator. vekst_migrator deliberately holds only
// CREATEROLE and ownership of its database (deploy/db/provision-migrator.sql):
// it is not a superuser, because a superuser bypasses row-level security
// unconditionally and CI would then prove nothing about a managed Postgres
// (design D0). Creating a database is something only this test harness does --
// production provisions one, out of band, once -- so it is the harness that
// carries the credential for it, and the documented contract stays the two
// privileges a hosted environment actually has to grant.
//
// The new database is owned by vekst_migrator: Postgres 15 and later make
// schema public owned by pg_database_owner, so that ownership is what gives
// migration 00001's REVOKE and ALTER DEFAULT PRIVILEGES something to act on.
// Without the OWNER clause the superuser would own it and 00001 would fail.
func testMigratorURL(t *testing.T, dbName string) string {
	t.Helper()
	base := os.Getenv("DATABASE_URL_MIGRATOR")
	if base == "" {
		t.Skip("DATABASE_URL_MIGRATOR not set; skipping a test that needs a live Postgres")
	}
	adminURL := os.Getenv("DATABASE_URL_ADMIN")
	if adminURL == "" {
		t.Skip("DATABASE_URL_ADMIN not set; skipping a test that needs to create a database")
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parsing DATABASE_URL_MIGRATOR: %v", err)
	}

	admin, err := sql.Open("pgx", adminURL)
	if err != nil {
		t.Fatalf("opening admin connection: %v", err)
	}
	// Closed inside Cleanup, after the DROP DATABASE it also does -- not a
	// plain defer here, which would close admin the moment this function
	// returns and make that later DROP silently fail with "database is
	// closed" (it did, the first time this was written).
	t.Cleanup(func() {
		defer admin.Close()
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			t.Logf("cleanup: dropping %s: %v", dbName, err)
		}
	})

	// DROP first: a previous run that panicked or was killed mid-test can
	// leave this behind, and CREATE DATABASE has no IF NOT EXISTS.
	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
		t.Fatalf("dropping stale test database: %v", err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + dbName + " OWNER vekst_migrator"); err != nil {
		t.Fatalf("creating test database: %v", err)
	}

	testURL := *u
	testURL.Path = "/" + dbName
	return testURL.String()
}

// Tasks 1.5 / 8.1's round trip, exercised through this package's own Up/Down
// rather than the vekst-core CLI -- and, as a side effect of goose running
// the registered Go migration, the only test coverage core/migrations/
// 00002_river.go's upRiver/downRiver get.
func TestUpDownUp(t *testing.T) {
	url := testMigratorURL(t, "vekst_migrate_updown_test")
	ctx := context.Background()

	if err := Up(ctx, url); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// goose_db_version + 5 River tables + 4 identity tables (00003) + 4
	// tenancy tables (00004).
	assertTableCount(t, url, 14)

	// Down four times: 00004 (tenancy), 00003 (identity), 00002 (River),
	// 00001 (roles), matching the four migrations actually registered.
	if err := Down(ctx, url); err != nil {
		t.Fatalf("Down (tenancy): %v", err)
	}
	if err := Down(ctx, url); err != nil {
		t.Fatalf("Down (identity): %v", err)
	}
	if err := Down(ctx, url); err != nil {
		t.Fatalf("Down (River): %v", err)
	}
	if err := Down(ctx, url); err != nil {
		t.Fatalf("Down (roles): %v", err)
	}
	// goose_db_version itself survives a full rollback -- goose needs
	// somewhere to record that the current version is 0. Confirmed
	// manually earlier in this change's own verification; asserting 0
	// here the first time was wrong, not the migration.
	assertTableCount(t, url, 1)

	if err := Up(ctx, url); err != nil {
		t.Fatalf("Up again: %v", err)
	}
	assertTableCount(t, url, 14)
}

func assertTableCount(t *testing.T, connURL string, want int) {
	t.Helper()
	db, err := sql.Open("pgx", connURL)
	if err != nil {
		t.Fatalf("opening connection: %v", err)
	}
	defer db.Close()

	var got int
	if err := db.QueryRow("SELECT count(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&got); err != nil {
		t.Fatalf("counting public tables: %v", err)
	}
	if got != want {
		t.Fatalf("public schema has %d table(s), want %d", got, want)
	}
}

func TestUpRejectsUnreachableDatabase(t *testing.T) {
	err := Up(context.Background(), "postgres://vekst_migrator@127.0.0.1:1/vekst?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("Up against an unreachable database: want an error")
	}
}
