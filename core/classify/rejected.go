package classify

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsRejected reports whether the engine refused the request itself, rather than
// failing to answer it.
//
// The distinction is the only thing a caller needs to decide between retrying
// and stopping, and it is not a judgement this package makes up: the engine's
// own service layer already draws it, and says so in the comment above its
// status table. `FAILED_PRECONDITION` is "fix the deployment" -- core
// normalised with a version this build does not implement. `INVALID_ARGUMENT`
// is "fix the caller" -- a rule pointing at a category the request did not
// carry, an operator no field supports. Neither is retryable, because a retry
// sends the identical request; that is the property the request carrying
// everything was arranged to have, and here it cuts the other way.
//
// Everything else -- unreachable, timed out, cancelled, unavailable -- is a
// condition that can pass, and a caller should let River try again.
//
// This lives here rather than in the worker because interpreting a transport
// status is what this package is for: it is the boundary, and the whole reason
// core/internal has no idea the engine speaks gRPC.
func IsRejected(err error) bool {
	if err == nil {
		return false
	}
	// status.FromError only unwraps one level; the client wraps with %w, so
	// walk the chain rather than testing the outermost error.
	for e := err; e != nil; e = errors.Unwrap(e) {
		s, ok := status.FromError(e)
		if !ok {
			continue
		}
		switch s.Code() {
		case codes.InvalidArgument, codes.FailedPrecondition:
			return true
		}
	}
	return false
}
