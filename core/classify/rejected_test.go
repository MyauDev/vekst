package classify_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/MyauDev/vekst/core/classify"
)

// Retry or stop, and nothing else depends on getting it right.
//
// A retry sends the identical request -- which is the property the request
// carrying everything was arranged to have, and it cuts both ways. When the
// engine merely did not answer, that makes a retry safe. When the engine
// answered by refusing, it makes one pointless: the same request will be
// refused again, forever, until River gives up and a batch that could have said
// why sits there saying nothing.
func TestIsRejectedSeparatesARefusalFromAnOutage(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want bool
	}{
		// The engine's own service layer maps its refusals onto these two, and
		// says in a comment why: "neither is retryable, which is what a River
		// worker needs to know".
		"a request the engine would not accept": {
			status.Error(codes.InvalidArgument, "unsupported_field"), true,
		},
		"a normalisation version it does not implement": {
			status.Error(codes.FailedPrecondition, "unsupported_normalize_version"), true,
		},

		// Everything else can pass on its own.
		"nobody listening":   {status.Error(codes.Unavailable, "connection refused"), false},
		"took too long":      {status.Error(codes.DeadlineExceeded, "context deadline exceeded"), false},
		"cancelled":          {status.Error(codes.Canceled, "context canceled"), false},
		"the engine crashed": {status.Error(codes.Internal, "panic"), false},

		// Not a gRPC error at all, and not nothing.
		"a plain error": {errors.New("something went wrong"), false},
		"no error":      {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := classify.IsRejected(c.err); got != c.want {
				t.Errorf("IsRejected(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// The client wraps what it gets with %w, so the status is never the outermost
// error by the time a worker sees it. A check that only looked at the top would
// treat every refusal as retryable and spend River's attempts discovering that
// the answer does not change.
func TestIsRejectedSeesThroughWrapping(t *testing.T) {
	inner := status.Error(codes.InvalidArgument, "rule names a category the request did not carry")
	wrapped := fmt.Errorf("classifier: classify batch %q: %w", "req-1", inner)
	doubly := fmt.Errorf("classifyrun: classifying a chunk of %d: %w", 5000, wrapped)

	for name, err := range map[string]error{
		"once":  wrapped,
		"twice": doubly,
	} {
		if !classify.IsRejected(err) {
			t.Errorf("a refusal wrapped %s was read as retryable: %v", name, err)
		}
	}

	// And wrapping does not make a retryable failure look final either.
	outage := fmt.Errorf("classifier: %w", status.Error(codes.Unavailable, "no route to host"))
	if classify.IsRejected(outage) {
		t.Error("an outage was read as a refusal; the batch would be marked failed for " +
			"something that passes on its own")
	}
}

// Unavailable is the classifier this repository uses wherever one is not
// configured, and what it returns must be retryable: a process started without
// an engine address is a deployment that can be fixed while jobs wait, not a
// reason to write every batch off.
func TestTheUnconfiguredClassifierIsRetryable(t *testing.T) {
	_, err := classify.Unavailable{}.Classify(context.Background(), classify.BatchRequest{})
	if err == nil {
		t.Fatal("Unavailable classified something")
	}
	if classify.IsRejected(err) {
		t.Error("an unconfigured classifier reads as a refusal, so every batch imported " +
			"before the engine is wired up would be marked failed permanently")
	}
}
