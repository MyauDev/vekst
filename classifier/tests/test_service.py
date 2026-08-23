"""Task 5.5: the service answers Version, and reports itself healthy."""

from collections.abc import Iterator

import grpc
import pytest
from grpc_health.v1 import health_pb2, health_pb2_grpc

from vekst.internal.v1 import classifier_pb2, classifier_pb2_grpc
from vekst_classifier.server import build_server


@pytest.fixture
def channel() -> Iterator[grpc.Channel]:
    """A real server on an ephemeral port. No network beyond loopback."""
    server = build_server("127.0.0.1:0")
    # add_insecure_port returns the bound port when given 0.
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as ch:
            yield ch
    finally:
        server.stop(None)


def test_version_is_populated(channel: grpc.Channel) -> None:
    stub = classifier_pb2_grpc.ClassifierServiceStub(channel)

    resp = stub.Version(classifier_pb2.VersionRequest())

    # "dev" is the unstamped value; empty would mean the stamping contract broke.
    assert resp.engine_version, "engine_version must never be empty"
    assert resp.built_at, "built_at must never be empty"


def test_health_service_reports_serving(channel: grpc.Channel) -> None:
    """Kubernetes probes this natively. No HTTP server exists to fall back on."""
    stub = health_pb2_grpc.HealthStub(channel)

    # HealthStub's methods are attached at runtime by grpc, so mypy cannot see
    # them. Ignored here rather than by relaxing strict mode for the package.
    overall = stub.Check(health_pb2.HealthCheckRequest())  # type: ignore[attr-defined]
    service = "vekst.internal.v1.ClassifierService"
    named = stub.Check(  # type: ignore[attr-defined]
        health_pb2.HealthCheckRequest(service=service)
    )

    assert overall.status == health_pb2.HealthCheckResponse.SERVING
    assert named.status == health_pb2.HealthCheckResponse.SERVING


def test_classify_batch_is_not_declared() -> None:
    """Change 3.2 owns ClassifyBatch. Declaring it early freezes guesses."""
    service = classifier_pb2.DESCRIPTOR.services_by_name["ClassifierService"]
    methods = {m.name for m in service.methods}

    assert methods == {"Version"}, f"unexpected RPCs on the contract: {methods}"
