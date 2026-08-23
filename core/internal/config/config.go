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

// Config is the whole of this service's configuration. Change 0.2 adds the
// database; until then there is deliberately nothing stateful here.
type Config struct {
	// Addr is the HTTP listen address, e.g. ":8080".
	Addr string

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
}

// Load reads configuration from the environment, applying defaults that are
// correct for local development.
func Load() (Config, error) {
	c := Config{
		Addr:              env("VEKST_ADDR", ":8080"),
		ClassifierAddr:    env("VEKST_CLASSIFIER_ADDR", ""),
		ClassifierTimeout: 2 * time.Second,
		ShutdownTimeout:   15 * time.Second,
		LogLevel:          env("VEKST_LOG_LEVEL", "info"),
	}

	var err error
	if c.ClassifierTimeout, err = envDuration("VEKST_CLASSIFIER_TIMEOUT", c.ClassifierTimeout); err != nil {
		return Config{}, err
	}
	if c.ShutdownTimeout, err = envDuration("VEKST_SHUTDOWN_TIMEOUT", c.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	return c, nil
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
