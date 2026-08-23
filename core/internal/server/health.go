package server

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	"github.com/MyauDev/vekst/core/classify"
	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/buildinfo"
)

// healthHandler implements vekst.v1.HealthService.
//
// It holds no database handle and no clock. It is the only unauthenticated RPC,
// and it reads no tenant data -- constraints every later change inherits.
type healthHandler struct {
	classifier classify.Classifier
	log        *slog.Logger
}

// Check reports core's own health, and best-effort reports the classifier's
// version alongside it.
//
// Core returns STATUS_SERVING whether or not the classifier answers. A
// classifier outage is a retryable condition, not an outage of core -- see
// ARCHITECTURE.md 3.5. Encoding that from the first commit is cheaper than
// discovering it during the first incident.
func (h *healthHandler) Check(
	ctx context.Context,
	_ *connect.Request[vektv1.CheckRequest],
) (*connect.Response[vektv1.CheckResponse], error) {
	resp := &vektv1.CheckResponse{
		Status:  vektv1.CheckResponse_STATUS_SERVING,
		Version: buildinfo.Version(),
		BuiltAt: buildinfo.BuiltAt(),
	}

	if info, err := h.classifier.Version(ctx); err != nil {
		// Deliberately not an error response. Logged at debug because an
		// unconfigured classifier is normal in unit tests and in `go run`.
		h.log.DebugContext(ctx, "classifier version unavailable", "err", err)
	} else {
		resp.ClassifierVersion = info.EngineVersion
	}

	return connect.NewResponse(resp), nil
}
