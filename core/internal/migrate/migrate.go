// Package migrate applies core's schema migrations and reports the version
// the running binary requires. Both go through the same embedded
// migrations.FS as the SQL migrations and the same goose global registry as
// the Go migration in core/migrations/00002_river.go, so "what does this
// binary need" (RequiredVersion, used by /readyz) and "what does this binary
// apply" (Up) can never drift apart -- openspec design Q5.
//
// This package opens its own *sql.DB via database/sql (goose's Go-migration
// signature requires it) rather than going through core/internal/db's pool.
// It is invoked once, at startup, before the server -- never from a request
// path -- so it is exempt from the one-transaction-entry-point rule design
// D2 enforces for request-serving code, and it authenticates as
// vekst_migrator, a credential the serving process never holds (design
// Q1/Q2).
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/MyauDev/vekst/core/migrations"
)

func init() {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		// A hardcoded, known-valid dialect string cannot fail here; a
		// failure would mean this package itself is broken.
		panic(fmt.Sprintf("migrate: %v", err))
	}
}

// RequiredVersion is the highest migration version this binary embeds and
// registers -- SQL migrations from migrations.FS and River's Go migration
// alike. It is never a separately declared constant that could disagree
// with the files: adding a migration file is what changes this number.
func RequiredVersion() (int64, error) {
	all, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("migrate: collecting migrations: %w", err)
	}
	last, err := all.Last()
	if err != nil {
		return 0, fmt.Errorf("migrate: no migrations registered: %w", err)
	}
	return last.Version, nil
}

// Up applies every pending migration. databaseURL must authenticate as
// vekst_migrator (design Q1/Q2); migration 00001 itself refuses to run as
// any other role.
func Up(ctx context.Context, databaseURL string) error {
	db, err := open(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.UpContext(ctx, db, ".")
}

// Down rolls back a single migration.
func Down(ctx context.Context, databaseURL string) error {
	db, err := open(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.DownContext(ctx, db, ".")
}

func open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrate: opening connection: %w", err)
	}
	return db, nil
}
