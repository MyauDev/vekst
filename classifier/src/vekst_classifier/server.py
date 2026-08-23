"""gRPC server entry point.

No HTTP server and no FastAPI. Kubernetes probes the standard gRPC health
protocol natively, so an HTTP port would exist with no caller. FastAPI earns its
place when something needs HTTP -- see design D2.
"""

import logging
import os
import signal
import sys
from concurrent import futures
from types import FrameType

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

from vekst.internal.v1 import classifier_pb2, classifier_pb2_grpc

from . import buildinfo
from .service import ClassifierService

_SERVICE_NAME = classifier_pb2.DESCRIPTOR.services_by_name[
    "ClassifierService"
].full_name
_SHUTDOWN_GRACE_SECONDS = 15

log = logging.getLogger("vekst.classifier")


def build_server(address: str, max_workers: int = 8) -> grpc.Server:
    """Construct the gRPC server. Performs no I/O beyond binding the port."""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))

    classifier_pb2_grpc.add_ClassifierServiceServicer_to_server(
        ClassifierService(), server
    )

    # The standard health service, so the Kubernetes gRPC probe works without
    # us inventing our own endpoint.
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    for name in ("", _SERVICE_NAME):
        health_servicer.set(name, health_pb2.HealthCheckResponse.SERVING)

    server.add_insecure_port(address)
    return server


def main() -> None:
    logging.basicConfig(
        level=os.environ.get("VEKST_LOG_LEVEL", "INFO").upper(),
        format='{"level":"%(levelname)s","msg":"%(message)s"}',
        stream=sys.stdout,
    )
    address = os.environ.get("VEKST_CLASSIFIER_ADDR", "[::]:9090")

    server = build_server(address)
    server.start()
    log.info(
        "listening on %s version=%s built_at=%s",
        address,
        buildinfo.engine_version(),
        buildinfo.built_at(),
    )

    def _stop(signum: int, _frame: FrameType | None) -> None:
        log.info("shutting down on signal %s", signum)
        server.stop(_SHUTDOWN_GRACE_SECONDS)

    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
