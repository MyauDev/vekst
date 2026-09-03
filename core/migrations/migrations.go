// Package migrations holds core's schema history: the SQL migrations goose
// applies directly, and the Go migration in 00002_river.go that wraps
// River's own migrator so `goose status` remains one ordered history rather
// than two (openspec design D1).
//
// FS is embedded so the binary that migrates and the binary that serves are
// the same binary reading the same files (design Q5): core/internal/migrate
// uses it to apply migrations, and core/internal/server's readiness check
// derives the schema version the binary requires from the same embedded
// filenames it lists here -- never a separately declared constant that could
// disagree with them.
//
// Because a Go migration (00002) must be compiled into the binary that runs
// it, plain SQL migrations are not enough here: the standalone `goose` CLI
// cannot see 00002's init() registration. Migrations run through
// core/internal/migrate, invoked as `vekst-core migrate up|down`, never
// through `go run .../goose/cmd/goose` directly.
package migrations

import "embed"

// FS embeds every *.sql migration in this directory. Go migrations
// (00002_river.go) register themselves via goose.AddMigrationNoTxContext in
// their own init() and are not part of this embed -- they are compiled in
// directly.
//
//go:embed *.sql
var FS embed.FS
