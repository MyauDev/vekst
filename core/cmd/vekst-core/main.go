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
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
	"github.com/MyauDev/vekst/core/internal/classifyrun"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/ingest"
	"github.com/MyauDev/vekst/core/internal/jobs"
	"github.com/MyauDev/vekst/core/internal/migrate"
	"github.com/MyauDev/vekst/core/internal/report"
	"github.com/MyauDev/vekst/core/internal/review"
	"github.com/MyauDev/vekst/core/internal/server"
	"github.com/MyauDev/vekst/core/internal/tenancy"
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

	// An unconfigured object store is a supported state too (add-file-upload
	// design D5, D-6 unanswered): store stays nil, CreateImportBatch answers
	// with a configuration error, and the workers that would need it are
	// registered regardless -- they never run, because no batch can be
	// created for them to act on.
	var store blob.ObjectStore
	if cfg.ObjectStoreConfigured() {
		store = blob.New(blob.Config{
			Endpoint:        cfg.ObjectStoreEndpoint,
			PresignEndpoint: cfg.ObjectStorePresignEndpoint,
			Bucket:          cfg.ObjectStoreBucket,
			Region:          cfg.ObjectStoreRegion,
			AccessKeyID:     cfg.ObjectStoreAccessKeyID,
			SecretKey:       cfg.ObjectStoreSecretKey,
			PathStyle:       cfg.ObjectStorePathStyle,
		})
		log.Info("object store configured", "endpoint", cfg.ObjectStoreEndpoint, "bucket", cfg.ObjectStoreBucket)
	} else {
		log.Warn("object store is not configured; CreateImportBatch will answer with " +
			ingest.ErrObjectStoreNotConfigured.Error())
	}
	ingestCfg := ingest.Config{
		UploadMaxBytes:    cfg.UploadMaxBytes,
		UploadURLLifetime: cfg.UploadURLLifetime,
	}
	ingestWorkers := ingest.NewWorkers(database, store, ingestCfg)

	// The job that turns persisted rows into classifications. It holds the
	// same Classifier the health check reports on: one client, one connection
	// to the engine, and no second place that decides what "the classifier" is.
	classifyWorkers := classifyrun.NewWorkers(database, classifier, classifyrun.Versions{
		Taxonomy: taxonomyVersion,
		Ruleset:  rulesetVersion,
	})

	jobsClient, err := jobs.New(database, ident, ingestWorkers, classifyWorkers)
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

	// Built after jobsClient, not passed into jobs.New: Service is the side
	// of this package that enqueues jobs, which needs the *jobs.Client
	// jobs.New returns, while Workers (above) is the side that only defines
	// them and needed nothing from it. See Workers' doc comment.
	importSvc := ingest.NewService(database, store, jobsClient, ingestCfg)

	return server.New(cfg, log, classifier, database, ident, importSvc,
		review.New(database, review.HumanVersions(taxonomyVersion, rulesetVersion)),
		report.New(database), tenancy.NewService(database)).Run(ctx)
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

// The taxonomy and rule set a human decision is recorded against. Constants
// until a customer can be on a version other than the one this binary seeded
// -- at which point they come from the organisation's own row, and this is the
// line that has to change.
const (
	taxonomyVersion = "v1"

	// v2 since migration 00019: v1's 71 template rules carried forward plus
	// three Belarusian revenue rules. v1 is left in place and still means what
	// it meant -- every classification written before the bump pins it, and a
	// report spanning the bump reports both, which is what ReportVersions
	// being a repeated field is for.
	rulesetVersion = "v2"
)
