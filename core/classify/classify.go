// Package classify holds the Classifier interface required by ARCHITECTURE.md
// A-2, and the gRPC client that implements it.
//
// Under design D2 the engine itself is a separate Python service from day one,
// so what lives here is the boundary, not the algorithm. The interface exists
// anyway: it is what lets core be tested without a classifier running, and it
// is the shape ARCHITECTURE.md 2.3 requires the engine to keep.
//
// Change 3.2 adds ClassifyBatch. This package deliberately has no database
// handle, no clock and no globals -- see ARCHITECTURE.md 2.3 and A-4.
package classify

import "context"

// VersionInfo is what the classifier reports about itself. engine_version is
// recorded on every classification row from change 3.2 onward.
type VersionInfo struct {
	EngineVersion string
	BuiltAt       string
}

// Classifier is the boundary between core and the classification engine.
//
// Change 3.2 adds:
//
//	Classify(ctx context.Context, req ClassifyBatchRequest) (ClassifyBatchResponse, error)
type Classifier interface {
	// Version reports the engine build. Implementations must respect ctx
	// deadlines: callers treat this as best-effort and must not be made to wait.
	Version(ctx context.Context) (VersionInfo, error)
}
