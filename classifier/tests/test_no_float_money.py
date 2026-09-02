"""Go no-float guard's Python counterpart (openspec design D5, task 5.6).

ARCHITECTURE.md 6 asks for this explicitly because pandas makes float the
default path. Walks every generated protobuf message under vekst/ -- not a
hand-picked list -- and fails if any field whose name marks it as money is a
float or double. Added now, before the classifier's contract carries its
first money field, because that is the only moment it costs nothing to
satisfy.
"""

import importlib
import pkgutil
import re
from types import ModuleType

from google.protobuf import descriptor as descriptor_module

import vekst

MONEY_FIELD_NAME = re.compile(r"(?i)^(amount|minor|money).*$|.*(amount|money)$")

FLOAT_TYPES = {
    descriptor_module.FieldDescriptor.TYPE_FLOAT,
    descriptor_module.FieldDescriptor.TYPE_DOUBLE,
}


def _pb2_modules() -> list[ModuleType]:
    """Every generated *_pb2 module reachable under the vekst namespace
    package -- vekst.internal.v1.classifier_pb2, vekst.type.v1.money_pb2,
    and any generated later, with nothing hand-maintained to fall out of
    date."""
    modules = []
    for module_info in pkgutil.walk_packages(vekst.__path__, prefix="vekst."):
        if not module_info.name.endswith("_pb2"):
            continue
        modules.append(importlib.import_module(module_info.name))
    return modules


def test_no_float_money_fields() -> None:
    violations = []

    for module in _pb2_modules():
        pool = module.DESCRIPTOR.pool
        for message_type in module.DESCRIPTOR.message_types_by_name.values():
            descriptor = pool.FindMessageTypeByName(message_type.full_name)
            for field in descriptor.fields:
                if field.type not in FLOAT_TYPES:
                    continue
                if MONEY_FIELD_NAME.match(field.name):
                    violations.append(f"{descriptor.full_name}.{field.name} is a float")

    assert not violations, "money fields must never be float:\n" + "\n".join(violations)
