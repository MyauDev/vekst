package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseMaxConns <= 0 || cfg.DatabaseConnectTimeout <= 0 {
		t.Errorf("database pool defaults must be positive, got %d and %v", cfg.DatabaseMaxConns, cfg.DatabaseConnectTimeout)
	}

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	// An unset classifier address is a supported state, not an error: core
	// serves and classifier_version comes back empty.
	if cfg.ClassifierAddr != "" {
		t.Errorf("ClassifierAddr = %q, want empty by default", cfg.ClassifierAddr)
	}
	if cfg.ClassifierTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		t.Errorf("timeouts must be positive, got %v and %v", cfg.ClassifierTimeout, cfg.ShutdownTimeout)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_ADDR", ":9999")
	t.Setenv("VEKST_CLASSIFIER_ADDR", "classifier:9090")
	t.Setenv("VEKST_CLASSIFIER_TIMEOUT", "750ms")
	t.Setenv("VEKST_SHUTDOWN_TIMEOUT", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.ClassifierAddr != "classifier:9090" {
		t.Errorf("ClassifierAddr = %q", cfg.ClassifierAddr)
	}
	if cfg.ClassifierTimeout != 750*time.Millisecond {
		t.Errorf("ClassifierTimeout = %v, want 750ms", cfg.ClassifierTimeout)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
}

// An empty value must not override the default. Kubernetes sets empty env vars
// readily -- an unset ConfigMap key arrives as "" -- and silently listening on
// "" instead of ":8080" would be very hard to trace back to its cause.
func TestEmptyEnvValueFallsBackToDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want the default when the variable is set but empty", cfg.Addr)
	}
}

// Fail at startup rather than run with a default somebody thought they had
// overridden. The negative case the spec asks every change to carry.
func TestInvalidDurationIsRejected(t *testing.T) {
	for name, value := range map[string]string{
		"not a duration": "soon",
		"bare number":    "30",
		"zero":           "0s",
		"negative":       "-5s",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("VEKST_SHUTDOWN_TIMEOUT", value)

			if _, err := Load(); err == nil {
				t.Errorf("Load() accepted %q; it must fail loudly at startup", value)
			}
		})
	}
}

func TestInvalidClassifierTimeoutIsRejected(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_CLASSIFIER_TIMEOUT", "never")

	if _, err := Load(); err == nil {
		t.Error("Load() accepted an unparseable classifier timeout")
	}
}

func TestInvalidDatabaseConnectTimeoutIsRejected(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_DB_CONNECT_TIMEOUT", "not-a-duration")

	if _, err := Load(); err == nil {
		t.Error("Load() accepted an unparseable VEKST_DB_CONNECT_TIMEOUT")
	}
}

// core always connects to Postgres from change 0.2 onward -- an unset
// DATABASE_URL must fail at startup, not surface as a mysterious dial error
// once the server is already listening.
func TestMissingDatabaseURLIsRejected(t *testing.T) {
	// t.Setenv, not an ambient assumption: a developer or CI job that
	// happens to have DATABASE_URL exported (e.g. while running the live
	// db/jobs tests elsewhere in this session) would otherwise make this
	// test silently pass for the wrong reason.
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Error("Load() accepted an unset DATABASE_URL")
	}
}
