// Command vekst-core is the Vekst backend: auth, tenancy, ingest, transactions,
// review, reports. It owns all state.
//
// The classification engine is not here -- it is a separate Python service
// reached over gRPC. See openspec design D2 and ARCHITECTURE.md 3.2.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/MyauDev/vekst/core/classify"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
	"github.com/MyauDev/vekst/core/internal/config"
	"github.com/MyauDev/vekst/core/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
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

	return server.New(cfg, log, classifier).Run(ctx)
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
