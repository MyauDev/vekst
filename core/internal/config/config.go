// Package config reads process configuration from the environment.
//
// Everything is read once, at startup, and passed down explicitly. No package
// reaches for an environment variable at call time -- the classification engine
// rule in ARCHITECTURE.md 2.3 (no globals, no hidden state) is easier to hold if
// the rest of the service works the same way.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the whole of this service's configuration.
type Config struct {
	// Addr is the HTTP listen address, e.g. ":8080".
	Addr string

	// DatabaseURL is a libpq connection string authenticating as vekst_app.
	// core never holds vekst_migrator credentials -- those belong to the
	// migration Job alone (design Q1/Q2). Read from a Secret named
	// vekst-db-app, key "url", in every environment but local.
	DatabaseURL string

	// DatabaseMaxConns bounds the connection pool.
	DatabaseMaxConns int32

	// DatabaseConnectTimeout bounds the initial connection and startup ping.
	DatabaseConnectTimeout time.Duration

	// ClassifierAddr is the internal gRPC target for the classifier service,
	// e.g. "classifier:9090". Empty disables the call: core still serves, and
	// classifier_version comes back empty. See ARCHITECTURE.md 3.5.
	ClassifierAddr string

	// ClassifierTimeout bounds a single call to the classifier. It is short on
	// purpose: this is a health lookup, not the classification path.
	ClassifierTimeout time.Duration

	// ShutdownTimeout bounds the graceful drain on SIGTERM.
	ShutdownTimeout time.Duration

	// LogLevel is one of debug, info, warn, error.
	LogLevel string

	// GoogleClientID and GoogleClientSecret authenticate core to Google's
	// OIDC endpoints. Both empty means sign-in is not configured: core still
	// serves, and the auth routes answer with a configuration error rather
	// than a nil dereference, so a developer with no credentials still gets a
	// working stack (add-identity design D6a).
	GoogleClientID     string
	GoogleClientSecret string

	// GoogleRedirectURL must exactly match a URI registered on the Google
	// OAuth client. Google refuses to register a host that is a subdomain of
	// localhost, which is why the local overlay serves plain "localhost"
	// (design D6).
	GoogleRedirectURL string

	// SessionLifetime is how long a session stays valid after sign-in.
	SessionLifetime time.Duration

	// SessionRetention is how long an expired session row survives before the
	// expiry job removes it. Expiry ends access; retention only governs how
	// long the row remains readable to an operator asking what happened.
	SessionRetention time.Duration

	// AuthFlowLifetime bounds how long a started sign-in may stay pending.
	// Short on purpose: it is the window in which a state value is live.
	AuthFlowLifetime time.Duration

}

// PlaceholderCredential is the value committed in
// deploy/k8s/overlays/local/google-oidc.yaml so that `kubectl kustomize`
// renders and CI stays green without a real secret in the repository. Real
// values arrive from a git-ignored google-oidc.secret.yaml that Tilt applies
// when it is present.
//
// core refuses to start when it sees this value. A placeholder that silently
// reaches a running service is worse than a missing one: sign-in would fail at
// Google with an opaque error, far from the cause.
const PlaceholderCredential = "REPLACE_ME"

// GoogleConfigured reports whether sign-in can work at all.
func (c Config) GoogleConfigured() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleRedirectURL != ""
}

// Load reads configuration from the environment, applying defaults that are
// correct for local development.
func Load() (Config, error) {
	c := Config{
		Addr:                   env("VEKST_ADDR", ":8080"),
		ClassifierAddr:         env("VEKST_CLASSIFIER_ADDR", ""),
		ClassifierTimeout:      2 * time.Second,
		ShutdownTimeout:        15 * time.Second,
		LogLevel:               env("VEKST_LOG_LEVEL", "info"),
		DatabaseURL:            env("DATABASE_URL", ""),
		DatabaseMaxConns:       10,
		DatabaseConnectTimeout: 5 * time.Second,
		GoogleClientID:         env("VEKST_GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:     env("VEKST_GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:      env("VEKST_GOOGLE_REDIRECT_URL", ""),
		SessionLifetime:        14 * 24 * time.Hour,
		SessionRetention:       7 * 24 * time.Hour,
		AuthFlowLifetime:       10 * time.Minute,
	}

	var err error
	if c.ClassifierTimeout, err = envDuration("VEKST_CLASSIFIER_TIMEOUT", c.ClassifierTimeout); err != nil {
		return Config{}, err
	}
	if c.ShutdownTimeout, err = envDuration("VEKST_SHUTDOWN_TIMEOUT", c.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if c.DatabaseConnectTimeout, err = envDuration("VEKST_DB_CONNECT_TIMEOUT", c.DatabaseConnectTimeout); err != nil {
		return Config{}, err
	}
	if c.SessionLifetime, err = envDuration("VEKST_SESSION_LIFETIME", c.SessionLifetime); err != nil {
		return Config{}, err
	}
	if c.SessionRetention, err = envDuration("VEKST_SESSION_RETENTION", c.SessionRetention); err != nil {
		return Config{}, err
	}
	if c.AuthFlowLifetime, err = envDuration("VEKST_AUTH_FLOW_LIFETIME", c.AuthFlowLifetime); err != nil {
		return Config{}, err
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is not set")
	}
	// Absent is a supported state; the placeholder is not. See
	// PlaceholderCredential.
	for _, f := range []struct{ name, value string }{
		{"VEKST_GOOGLE_CLIENT_ID", c.GoogleClientID},
		{"VEKST_GOOGLE_CLIENT_SECRET", c.GoogleClientSecret},
		{"VEKST_GOOGLE_REDIRECT_URL", c.GoogleRedirectURL},
	} {
		if f.value == PlaceholderCredential {
			return Config{}, fmt.Errorf(
				"%s is still %q: apply deploy/k8s/overlays/local/google-oidc.secret.yaml with real values, or unset it to run without sign-in",
				f.name, PlaceholderCredential)
		}
	}
	return c, nil
}

func envBool(key string, def bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %q is not a boolean: %w", key, raw, err)
	}
	return v, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		// Fail loudly at startup rather than silently running with a default
		// somebody thought they had overridden.
		return 0, fmt.Errorf("%s: %q is not a duration: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: must be positive, got %s", key, strconv.Quote(raw))
	}
	return d, nil
}
