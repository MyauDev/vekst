package db

import (
	"context"
	"testing"
	"time"
)

func TestNewRejectsEmptyURL(t *testing.T) {
	if _, err := New(context.Background(), Config{}); err == nil {
		t.Fatal("New with an empty URL: want an error")
	}
}

func TestNewRejectsUnparseableURL(t *testing.T) {
	if _, err := New(context.Background(), Config{URL: "not a connection string"}); err == nil {
		t.Fatal("New with an unparseable URL: want an error")
	}
}

func TestNewFailsFastWhenUnreachable(t *testing.T) {
	// Port 1 is a reserved, never-listening TCP port -- fails immediately
	// rather than waiting out a real connect timeout.
	cfg := Config{
		URL:            "postgres://vekst_app@127.0.0.1:1/vekst?sslmode=disable",
		ConnectTimeout: 500 * time.Millisecond,
	}
	if _, err := New(context.Background(), cfg); err == nil {
		t.Fatal("New against an unreachable database: want an error")
	}
}

func TestPoolExposesTheRawPool(t *testing.T) {
	d := testDB(t)
	if d.Pool() == nil {
		t.Fatal("Pool() returned nil on a constructed *DB")
	}
}

func TestPingSucceedsAgainstALiveDatabase(t *testing.T) {
	d := testDB(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAppliedMigrationVersionReadsGooseDBVersion(t *testing.T) {
	d := testDB(t)
	version, err := d.AppliedMigrationVersion(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrationVersion: %v", err)
	}
	if version == 0 {
		t.Fatal("AppliedMigrationVersion returned 0 against a migrated database")
	}
}
