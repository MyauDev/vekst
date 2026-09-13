"""The ClassifierService gRPC implementation.

Change 3.2 adds ClassifyBatch. L2 and above remain future work -- see
proto/vekst/internal/v1/README.md.

This module holds no database handle, no clock it did not receive, and no
global state. ARCHITECTURE.md 2.3 requires the engine to be a pure function of
its inputs; under design D2 it runs in its own process, so that property is
structural rather than a rule somebody has to remember. The servicer keeps it
that way by doing nothing but translating: a request in, engine.classify_batch,
a response or a status code out.
"""

import grpc

from vekst.internal.v1 import classifier_pb2, classifier_pb2_grpc

from . import buildinfo, engine

# Which status a refusal deserves. FAILED_PRECONDITION says "fix the
# deployment": core normalised with a version this build does not implement,
# and retrying the same request against the same build cannot help. The rest
# are INVALID_ARGUMENT -- core built a request it could not have meant, and a
# retry is equally pointless, but the fix is in the caller rather than in the
# fleet. Neither is retryable, which is what a River worker needs to know.
_STATUS = {
    "unsupported_normalize_version": grpc.StatusCode.FAILED_PRECONDITION,
}
_DEFAULT_STATUS = grpc.StatusCode.INVALID_ARGUMENT


class ClassifierService(classifier_pb2_grpc.ClassifierServiceServicer):
    """Implements vekst.internal.v1.ClassifierService."""

    def Version(
        self,
        request: classifier_pb2.VersionRequest,
        context: grpc.ServicerContext,
    ) -> classifier_pb2.VersionResponse:
        """Report the engine build. Called by core, never by a browser."""
        return classifier_pb2.VersionResponse(
            engine_version=buildinfo.engine_version(),
            built_at=buildinfo.built_at(),
        )

    def ClassifyBatch(
        self,
        request: classifier_pb2.ClassifyBatchRequest,
        context: grpc.ServicerContext,
    ) -> classifier_pb2.ClassifyBatchResponse:
        """Classify a chunk of transactions. Called by core, never by a browser.

        Everything the answer depends on arrives in the request. This service
        never had database credentials to lose (ARCHITECTURE.md A-4), so a
        tenant boundary cannot be crossed here by a bug: it was never
        reachable.
        """
        try:
            return engine.classify_batch(request, buildinfo.engine_version())
        except engine.RequestRejectedError as rejected:
            # The code, not a sentence: the backend returns codes and the
            # client translates (CLAUDE.md, Conventions). The detail goes into
            # the status message for a developer reading a failed job.
            context.abort(
                _STATUS.get(rejected.code, _DEFAULT_STATUS),
                f"{rejected.code}: {rejected.detail}",
            )
            raise  # unreachable: abort() raises. Present so mypy sees it.
