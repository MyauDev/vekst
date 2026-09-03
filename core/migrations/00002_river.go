package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

func init() {
	// NoTx: rivermigrate.Migrate manages its own transaction per River
	// migration step. Running it inside goose's own transaction as well
	// would nest transactions, which Postgres does not support.
	goose.AddMigrationNoTxContext(upRiver, downRiver)
}

// River's own schema: river_job (plus the river_job_state enum), river_leader,
// river_queue, river_notification and River's version table, river_migration.
// Wrapped in a goose migration so `goose status` remains the single answer to
// what has been applied, rather than two separate migration histories
// (openspec design D1, migration 002).
//
// River's migrator runs on database/sql (riverdatabasesql), matching goose's
// own Go-migration signature; the running application keeps pgx/v5
// throughout (core/internal/db, core/internal/jobs) -- confirmed per design
// D8's open verification note.
//
// None of River's tables carry org_id or an RLS policy: they are
// infrastructure, not tenant data (design D4). See
// deploy/db/rls-exempt-tables.txt, confirmed against the River version
// pinned in go.mod, and task 4.7b's test that the two stay in sync.
//
// vekst_app needs no separate GRANT here: migration 00001's
// ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public is a
// standing catalog rule, not a one-time statement, so any table vekst_migrator
// creates afterward -- including these, created by rivermigrate running as
// vekst_migrator via the DATABASE_URL_MIGRATOR connection migrate.Up opens --
// is usable by vekst_app immediately (design task 4.2).
func upRiver(ctx context.Context, db *sql.DB) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(db), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func downRiver(ctx context.Context, db *sql.DB) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(db), nil)
	if err != nil {
		return err
	}
	// rivermigrate defaults to a single down step "for safety" -- confirmed
	// against a real database while writing this, where it left five of
	// River's six migrations applied. This goose migration is one atomic
	// unit undoing everything upRiver did, so TargetVersion: -1 (rivermigrate's
	// documented value for "remove River's schema completely") is not
	// optional here.
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{TargetVersion: -1})
	return err
}
