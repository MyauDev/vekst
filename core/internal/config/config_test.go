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

// Absent object-store configuration is a supported state (add-file-upload
// design D5, D-6 unanswered): core serves, and only CreateImportBatch fails.
func TestLoadWithoutObjectStore(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load with no object store configured: %v", err)
	}
	if c.ObjectStoreConfigured() {
		t.Error("ObjectStoreConfigured() = true with nothing set")
	}
	if c.UploadMaxBytes != 26_214_400 {
		t.Errorf("UploadMaxBytes = %d, want the 25 MiB default", c.UploadMaxBytes)
	}
	if c.UploadURLLifetime != 15*time.Minute {
		t.Errorf("UploadURLLifetime = %v, want 15m", c.UploadURLLifetime)
	}
	if !c.ObjectStorePathStyle {
		t.Error("ObjectStorePathStyle default should be true, matching the local MinIO overlay")
	}
}

func TestLoadReadsObjectStoreEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
	t.Setenv("VEKST_OBJECT_STORE_ENDPOINT", "http://minio:9000")
	t.Setenv("VEKST_OBJECT_STORE_BUCKET", "vekst-test")
	t.Setenv("VEKST_UPLOAD_MAX_BYTES", "1048576")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.ObjectStoreConfigured() {
		t.Error("ObjectStoreConfigured() = false with an endpoint set")
	}
	if c.ObjectStoreBucket != "vekst-test" {
		t.Errorf("ObjectStoreBucket = %q", c.ObjectStoreBucket)
	}
	if c.UploadMaxBytes != 1048576 {
		t.Errorf("UploadMaxBytes = %d, want 1048576", c.UploadMaxBytes)
	}
}

func TestInvalidUploadMaxBytesIsRejected(t *testing.T) {
	for name, value := range map[string]string{
		"not a number": "soon",
		"zero":         "0",
		"negative":     "-5",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://vekst_app@localhost:5432/vekst")
			t.Setenv("VEKST_UPLOAD_MAX_BYTES", value)

			if _, err := Load(); err == nil {
				t.Errorf("Load() accepted VEKST_UPLOAD_MAX_BYTES=%q", value)
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
	if c.SessionLifetime <= 0 || c.AuthFlowLifetime <= 0 {
		t.Errorf("lifetimes must be positive: session=%s flow=%s", c.SessionLifetime, c.AuthFlowLifetime)
	}
}
