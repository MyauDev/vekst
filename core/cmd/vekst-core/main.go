// Command vekst-core is the Vekst backend: auth, tenancy, ingest, transactions,
// review, reports. It owns all state.
//
// The classification engine is not here -- it is a separate Python service
// reached over gRPC. See openspec design D2 and ARCHITECTURE.md 3.2.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/MyauDev/vekst/core/classify"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/jobs"
	"github.com/MyauDev/vekst/core/internal/migrate"
	"github.com/MyauDev/vekst/core/internal/server"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		err = runMigrate(os.Args[2:])
	} else {
		err = run()
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// runMigrate applies or rolls back schema migrations. It is the same binary
// that serves traffic, reading the same embedded migrations -- design Q5 --
// so the migration Job in deploy/k8s/base runs this image with a "migrate"
// argument rather than a separate image. Authenticates as vekst_migrator via
// DATABASE_URL_MIGRATOR, a credential this process's serving mode never
// reads (design Q1/Q2).
func runMigrate(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "down") {
		return fmt.Errorf("usage: vekst-core migrate up|down")
	}

	dsn := os.Getenv("DATABASE_URL_MIGRATOR")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL_MIGRATOR is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if args[0] == "up" {
		return migrate.Up(ctx, dsn)
	}
	return migrate.Down(ctx, dsn)
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level(cfg.LogLevel)}))
	slog.SetDefault(log)
	log.Info("starting",
		"version", buildinfo.Version(),
		"built_at", buildinfo.BuiltAt(),
		"classifier_addr", cfg.ClassifierAddr,
	)

	// An unconfigured classifier is a supported state, not an error: core serves
	// and classifier_version comes back empty. Using a real implementation that
	// always fails -- rather than a nil check -- keeps the unreachable path the
	// same code in every environment.
	var classifier classify.Classifier = classify.Unavailable{}
	if cfg.ClassifierAddr != "" {
		c, err := classify.Dial(cfg.ClassifierAddr)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		classifier = c
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.New(ctx, db.Config{
		URL:            cfg.DatabaseURL,
		MaxConns:       cfg.DatabaseMaxConns,
		ConnectTimeout: cfg.DatabaseConnectTimeout,
	})
	if err != nil {
		return err
	}
	defer database.Close()

	// Sign-in is optional. With no OAuth client configured core still serves
	// and only the auth routes fail, so a developer without credentials gets a
	// working stack (add-identity design D6a). Discovery of Google's endpoints
	// and JWKS happens here, once, with a bounded timeout -- never on the
	// first browser redirect.
	var ident *identity.Service
	if cfg.GoogleConfigured() {
		discoverCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
		ident, err = identity.New(discoverCtx, identity.Config{
			ClientID:         cfg.GoogleClientID,
			ClientSecret:     cfg.GoogleClientSecret,
			RedirectURL:      cfg.GoogleRedirectURL,
			SessionLifetime:  cfg.SessionLifetime,
			SessionRetention: cfg.SessionRetention,
			AuthFlowLifetime: cfg.AuthFlowLifetime,
			CookieSecure:     cfg.CookieSecure,
		}, database, log)
		cancel()
		if err != nil {
			return err
		}
		log.Info("sign-in configured", "redirect_url", cfg.GoogleRedirectURL)
	} else {
		log.Warn("sign-in is not configured; /auth routes will answer with " +
			identity.CodeNotConfigured)
	}

	jobsClient, err := jobs.New(database.Pool(), ident)
	if err != nil {
		return err
	}
	if err := jobsClient.Start(ctx); err != nil {
		return err
	}
	defer func() {
		// Bounded, like the HTTP drain below: a job that will not finish
		// must not hang shutdown forever.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := jobsClient.Stop(shutdownCtx); err != nil {
			log.Error("stopping river client", "err", err)
		}
	}()

	return server.New(cfg, log, classifier, database, ident).Run(ctx)
}

func level(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
