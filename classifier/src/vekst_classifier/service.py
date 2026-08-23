"""The ClassifierService gRPC implementation.

Change 0.1 implements Version only. ClassifyBatch and engine layers L0-L2 are
change 3.2's work -- see proto/vekst/internal/v1/README.md.

This module holds no database handle, no clock it did not receive, and no
global state. ARCHITECTURE.md 2.3 requires the engine to be a pure function of
its inputs; under design D2 it runs in its own process, so that property is
structural rather than a rule somebody has to remember.
"""

import grpc

from vekst.internal.v1 import classifier_pb2, classifier_pb2_grpc

from . import buildinfo


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
