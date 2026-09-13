// Package classify holds the Classifier interface required by ARCHITECTURE.md
// A-2, and the gRPC client that implements it.
//
// Under design D2 the engine itself is a separate Python service from day one,
// so what lives here is the boundary, not the algorithm. The interface exists
// anyway: it is what lets core be tested without a classifier running, and it
// is the shape ARCHITECTURE.md 2.3 requires the engine to keep.
//
// This package deliberately has no database handle, no clock and no globals --
// see ARCHITECTURE.md 2.3 and A-4.
package classify

import "context"

// VersionInfo is what the classifier reports about itself. engine_version is
// recorded on every classification row from change 3.2 onward.
type VersionInfo struct {
	EngineVersion string
	BuiltAt       string
}

// Classifier is the boundary between core and the classification engine.
type Classifier interface {
	// Version reports the engine build. Implementations must respect ctx
	// deadlines: callers treat this as best-effort and must not be made to wait.
	Version(ctx context.Context) (VersionInfo, error)

	// Classify answers one chunk of transactions.
	//
	// The request carries everything the answer depends on, so calling this
	// twice with the same argument returns the same proposals -- which is
	// what makes a retried job safe and a March report reproducible. Like
	// Version, implementations must respect ctx deadlines: a classifier that
	// is not answering is a retryable condition, not a reason to hang a
	// worker.
	Classify(ctx context.Context, req BatchRequest) (BatchResponse, error)
}
