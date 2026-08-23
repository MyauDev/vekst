package main

import (
	"log/slog"
	"testing"
)

func TestLevelParsing(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		// Anything unrecognised falls back to info rather than failing to
		// start. A typo in a log level should not take the service down.
		"":        slog.LevelInfo,
		"verbose": slog.LevelInfo,
		"DEBUG":   slog.LevelInfo, // case-sensitive by design; documented default
	} {
		if got := level(in); got != want {
			t.Errorf("level(%q) = %v, want %v", in, got, want)
		}
	}
}
