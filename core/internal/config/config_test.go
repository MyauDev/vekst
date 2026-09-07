package config

import (
	"strings"
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

// Absent Google credentials are a supported state: core serves, and only the
// sign-in routes fail. A developer with no OAuth client still gets a working
// stack (add-identity design D6a).
func TestLoadWithoutGoogleCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load with no Google credentials: %v", err)
	}
	if c.GoogleConfigured() {
		t.Error("GoogleConfigured() = true with nothing set")
	}
}

// The placeholder is not a supported state. It is committed so that
// `kubectl kustomize` renders every overlay in CI, and reaching a running
// service means the real secret was never applied -- which would otherwise
// surface as an opaque failure at Google, far from its cause.
func TestLoadRejectsPlaceholderCredentials(t *testing.T) {
	for _, key := range []string{
		"VEKST_GOOGLE_CLIENT_ID",
		"VEKST_GOOGLE_CLIENT_SECRET",
		"VEKST_GOOGLE_REDIRECT_URL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
			t.Setenv(key, PlaceholderCredential)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load with %s=%s: want an error", key, PlaceholderCredential)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error does not name the offending variable: %v", err)
			}
		})
	}
}

func TestLoadAcceptsRealGoogleCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_GOOGLE_CLIENT_ID", "123.apps.googleusercontent.com")
	t.Setenv("VEKST_GOOGLE_CLIENT_SECRET", "a-real-looking-secret")
	t.Setenv("VEKST_GOOGLE_REDIRECT_URL", "http://localhost:8081/auth/google/callback")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.GoogleConfigured() {
		t.Error("GoogleConfigured() = false with all three set")
	}
	if !c.CookieSecure {
		t.Error("CookieSecure defaults to false; it must default to true")
	}
	if c.SessionLifetime <= 0 || c.AuthFlowLifetime <= 0 {
		t.Errorf("lifetimes must be positive: session=%s flow=%s", c.SessionLifetime, c.AuthFlowLifetime)
	}
}

func TestLoadRejectsUnparsableCookieSecure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_COOKIE_SECURE", "yes-please")

	if _, err := Load(); err == nil {
		t.Fatal("VEKST_COOKIE_SECURE=yes-please: want an error")
	}
}
