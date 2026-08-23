package buildinfo

import (
	"testing"
	"time"
)

// Spec: "A build identifies itself" -- scenario "An unstamped build is
// identifiable". An empty string here would look like a value nobody set;
// "dev" says plainly that the stamp is missing.
func TestUnstampedBuildReportsDev(t *testing.T) {
	t.Cleanup(func() { version, builtAt = "", "" })
	version, builtAt = "", ""

	if got := Version(); got != "dev" {
		t.Errorf("Version() = %q, want %q", got, "dev")
	}
	if got := BuiltAt(); got != "dev" {
		t.Errorf("BuiltAt() = %q, want %q", got, "dev")
	}
}

// Spec: "Version reflects the built commit".
func TestStampedBuildReportsItsStamp(t *testing.T) {
	t.Cleanup(func() { version, builtAt = "", "" })
	version, builtAt = "691f6a5", "2026-08-23T12:14:45Z"

	if got := Version(); got != "691f6a5" {
		t.Errorf("Version() = %q, want the injected SHA", got)
	}
	if got := BuiltAt(); got != "2026-08-23T12:14:45Z" {
		t.Errorf("BuiltAt() = %q, want the injected time", got)
	}
	if _, err := time.Parse(time.RFC3339, BuiltAt()); err != nil {
		t.Errorf("BuiltAt() is not RFC 3339: %v", err)
	}
}

func TestStartedAtIsUTC(t *testing.T) {
	if StartedAt.Location() != time.UTC {
		t.Errorf("StartedAt is %v, want UTC; a local timestamp is ambiguous in logs", StartedAt.Location())
	}
}
