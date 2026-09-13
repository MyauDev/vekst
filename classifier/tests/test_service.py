"""Task 5.5: the service answers Version, and reports itself healthy."""

from collections.abc import Iterator

import grpc
import pytest
from google.protobuf.descriptor import FieldDescriptor
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


def test_the_contract_declares_exactly_two_rpcs() -> None:
    """The stop-cock moves, it does not disappear.

    Before change 3.2 this asserted that ClassifyBatch was absent, so that
    nobody froze a guess at its shape. Now that it exists the same assertion
    does the opposite job: an RPC added to this internal contract without a
    change behind it fails here, which is the only place a reviewer is
    guaranteed to look.
    """
    service = classifier_pb2.DESCRIPTOR.services_by_name["ClassifierService"]
    methods = {m.name for m in service.methods}

    assert methods == {
        "Version",
        "ClassifyBatch",
    }, f"unexpected RPCs on the contract: {methods}"


def test_money_fields_are_never_floating_point() -> None:
    """Task 6.3, asserted on the generated types rather than on the source.

    A `double amount` in the .proto would pass every lint and every review that
    reads the field name instead of the type. It cannot pass this: every field
    whose type is Money is a message, and no field anywhere in the contract is
    a float except the two the design names.
    """
    float_kinds = {
        FieldDescriptor.TYPE_DOUBLE,
        FieldDescriptor.TYPE_FLOAT,
    }
    allowed = {
        "vekst.internal.v1.ClassifyBatchRequest.threshold",
        "vekst.internal.v1.Proposal.confidence",
    }

    offenders = {
        field.full_name
        for message in classifier_pb2.DESCRIPTOR.message_types_by_name.values()
        for field in message.fields
        if field.type in float_kinds and field.full_name not in allowed
    }

    assert not offenders, f"floating-point fields on the contract: {offenders}"


def test_money_carrying_fields_use_the_money_message() -> None:
    """The other half of the same guard.

    The test above catches a float where money belongs. This one catches money
    modelled as a string or an int64 without its currency -- an amount without
    a currency code is not an amount, and a report that adds two of them prints
    a wrong number rather than failing.
    """
    money = "vekst.type.v1.Money"
    expected = {
        "vekst.internal.v1.Condition.amount_value": money,
        "vekst.internal.v1.TxnForClassify.amount": money,
    }

    actual = {
        field.full_name: (
            field.message_type.full_name if field.message_type else str(field.type)
        )
        for message in classifier_pb2.DESCRIPTOR.message_types_by_name.values()
        for field in message.fields
        if field.name in {"amount", "amount_value"}
    }

    assert actual == expected
