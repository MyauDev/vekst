// Package buildinfo carries the identity of the running build.
//
// Values are injected at link time by -ldflags (see deploy/docker/Dockerfile.core
// and the Makefile). An unstamped build -- `go run`, `go test` -- reports "dev"
// rather than an empty string, so a missing stamp is visible instead of looking
// like a value nobody set.
package buildinfo

import "time"

// Injected via -ldflags -X. Do not set these anywhere else.
var (
	version = ""
	builtAt = ""
)

// Version returns the git SHA of this build, or "dev" when unstamped.
func Version() string {
	if version == "" {
		return "dev"
	}
	return version
}

// BuiltAt returns the build time as RFC 3339, or "dev" when unstamped.
func BuiltAt() string {
	if builtAt == "" {
		return "dev"
	}
	return builtAt
}

// StartedAt is when this process began. Not the build time; used for uptime.
var StartedAt = time.Now().UTC()
