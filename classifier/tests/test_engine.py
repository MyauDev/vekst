"""The engine's behaviour, pinned to the reference and to the design's rules.

`test_matches_the_reference_engine` is the one that matters most: it replays
`eval/out/engine_conformance.json`, which `eval/conformance.py` produces by
running `eval/engine.py` -- the code `eval/run_eval.py` measures -- over the
redacted Priorbank fixtures and a handful of synthetic rows. If this engine and
that one ever disagree, the coverage numbers in the design stop describing what
actually runs, and that is the failure this file exists to prevent.

The rest are the cases a corpus of one Belarusian company cannot contain:
currencies with a different exponent, a ledger row, a rule matching on an
amount.
"""

import json
import pathlib
from typing import Any

import pytest

from vekst.internal.v1 import classifier_pb2
from vekst.type.v1 import money_pb2
from vekst_classifier import engine

REPO = pathlib.Path(__file__).resolve().parents[2]
FIXTURE = REPO / "eval" / "out" / "engine_conformance.json"
RULES = REPO / "eval" / "out" / "rules.json"
CATEGORIES = REPO / "eval" / "out" / "categories.json"

ENGINE_VERSION = "test-engine"


def _load(path: pathlib.Path) -> dict[str, Any]:
    loaded: dict[str, Any] = json.loads(path.read_text(encoding="utf-8"))
    return loaded


def seeded_rules() -> list[classifier_pb2.Rule]:
    """The 71 template rules, as core would send them."""
    return [
        classifier_pb2.Rule(
            priority=r["priority"],
            category_code=r["category_code"],
            scope=r["scope"],
            source_kind=r["matcher"]["source_kind"],
            all=[
                classifier_pb2.Condition(field=c["field"], op=c["op"], value=c["value"])
                for c in r["matcher"]["all"]
            ],
        )
        for r in _load(RULES)["rules"]
    ]


def seeded_categories() -> list[classifier_pb2.Category]:
    return [
        classifier_pb2.Category(code=code, name=str(name))
        for code, name in _load(CATEGORIES).items()
    ]


def request(**kwargs: object) -> classifier_pb2.ClassifyBatchRequest:
    """A request with the boring parts filled in."""
    fields: dict[str, Any] = {
        "request_id": "test",
        "taxonomy_version": "v1",
        "ruleset_version": "v1",
        "normalize_version": "v1",
        "categories": seeded_categories(),
        "rules": seeded_rules(),
        "threshold": 0.80,
    }
    fields.update(kwargs)
    return classifier_pb2.ClassifyBatchRequest(**fields)


# ---------------------------------------------------------------------------
# Task 6.1
# ---------------------------------------------------------------------------


def test_matches_the_reference_engine() -> None:
    fixture = _load(FIXTURE)
    cases = fixture["cases"]
    assert cases, "the fixture is empty, so this test compares nothing"

    txns = [
        classifier_pb2.TxnForClassify(transaction_id=c["id"], **c["txn"]) for c in cases
    ]
    # Every case carries its own memory, so they are grouped by it rather than
    # sent as one batch: memory is per organisation, and a batch is too.
    by_memory: dict[str, list[int]] = {}
    for i, c in enumerate(cases):
        by_memory.setdefault(json.dumps(c["memory"], sort_keys=True), []).append(i)

    got: dict[str, classifier_pb2.Proposal] = {}
    for memory_json, indices in by_memory.items():
        memory = json.loads(memory_json)
        response = engine.classify_batch(
            request(
                txns=[txns[i] for i in indices],
                vendors=[
                    classifier_pb2.VendorMemory(
                        key=key, category_code=code, display_name=key
                    )
                    for key, code in memory.items()
                ],
            ),
            ENGINE_VERSION,
        )
        for p in response.proposals:
            got[p.transaction_id] = p

    mismatches = []
    for c in cases:
        want, have = c["expected"], got.get(c["id"])
        if want is None:
            if have is not None:
                mismatches.append(f"{c['id']}: expected no answer, got {have}")
            continue
        if have is None:
            mismatches.append(f"{c['id']}: expected {want}, got no answer")
            continue
        actual = {
            "category_code": have.category_code,
            "engine_layer": have.engine_layer,
            "confidence": round(have.confidence, 4),
            "evidence": have.evidence,
            "matched_rule_priority": have.matched_rule_priority,
        }
        if actual != {**want, "confidence": round(want["confidence"], 4)}:
            mismatches.append(f"{c['id']}: expected {want}, got {actual}")

    assert not mismatches, "\n".join(mismatches[:20])


def test_the_fixture_exercises_every_layer() -> None:
    """A replay that never reached L0 or L0.5 would pass while proving nothing."""
    layers = {
        c["expected"]["engine_layer"] for c in _load(FIXTURE)["cases"] if c["expected"]
    }
    assert layers == {"L0", "L0.5", "L1"}


# ---------------------------------------------------------------------------
# Task 6.2
# ---------------------------------------------------------------------------


def test_the_same_request_twice_is_byte_identical() -> None:
    req = request(
        txns=[
            classifier_pb2.TxnForClassify(
                transaction_id=str(i),
                source_kind="bank",
                description_norm="ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ",
                direction="expense",
            )
            for i in range(20)
        ]
    )
    first = engine.classify_batch(req, ENGINE_VERSION)
    second = engine.classify_batch(req, ENGINE_VERSION)

    assert first.SerializeToString(deterministic=True) == second.SerializeToString(
        deterministic=True
    )


# ---------------------------------------------------------------------------
# Task 6.4 -- money
# ---------------------------------------------------------------------------


def amount_rule(currency: str, minor_units: int, op: str) -> classifier_pb2.Rule:
    return classifier_pb2.Rule(
        priority=1,
        category_code="09",
        scope="org",
        source_kind="bank",
        all=[
            classifier_pb2.Condition(
                field="amount",
                op=op,
                amount_value=money_pb2.Money(
                    currency_code=currency, minor_units=minor_units
                ),
            )
        ],
    )


def txn_worth(currency: str, minor_units: int) -> classifier_pb2.TxnForClassify:
    return classifier_pb2.TxnForClassify(
        transaction_id="t",
        source_kind="bank",
        direction="expense",
        amount=money_pb2.Money(currency_code=currency, minor_units=minor_units),
    )


@pytest.mark.parametrize(
    ("currency", "threshold_units", "txn_units", "fires"),
    [
        # JPY has exponent 0: 1000 minor units is ¥1000.
        ("JPY", 1000, 1000, True),
        ("JPY", 1000, 999, False),
        # KWD has exponent 3: 1000 minor units is 1.000 KWD. The engine never
        # sees an exponent -- it compares integers of one currency -- which is
        # exactly why neither of these needs a special case.
        ("KWD", 1000, 1000, True),
        ("KWD", 1000, 999, False),
    ],
)
def test_amount_rules_are_integer_comparisons(
    currency: str, threshold_units: int, txn_units: int, fires: bool
) -> None:
    response = engine.classify_batch(
        request(
            rules=[amount_rule(currency, threshold_units, "gte")],
            txns=[txn_worth(currency, txn_units)],
        ),
        ENGINE_VERSION,
    )
    assert bool(response.proposals) is fires


def test_two_currencies_never_compare() -> None:
    """A rule written in EUR says nothing about a yen amount.

    Not an error and not a conversion: an FX rate has a date and the engine
    has no clock. The row falls through to the next rule, and to review if
    there is none.
    """
    response = engine.classify_batch(
        request(
            rules=[amount_rule("EUR", 1, "gte")],
            # Numerically far above the threshold, and still no match.
            txns=[txn_worth("JPY", 1_000_000)],
        ),
        ENGINE_VERSION,
    )
    assert list(response.proposals) == []


# ---------------------------------------------------------------------------
# Task 6.7 -- source kind
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("rule_kind", "txn_kind", "fires"),
    [
        ("bank", "bank", True),
        ("bank", "ledger", False),
        ("ledger", "ledger", True),
        ("ledger", "bank", False),
        # A rule that does not say is tried on both.
        ("", "bank", True),
        ("", "ledger", True),
    ],
)
def test_a_rule_fires_only_on_its_own_source_kind(
    rule_kind: str, txn_kind: str, fires: bool
) -> None:
    rule = classifier_pb2.Rule(
        priority=1,
        category_code="09",
        scope="org",
        source_kind=rule_kind,
        all=[
            classifier_pb2.Condition(
                field="description", op="contains_all", value="ДИВИДЕНДЫ"
            )
        ],
    )
    response = engine.classify_batch(
        request(
            rules=[rule],
            txns=[
                classifier_pb2.TxnForClassify(
                    transaction_id="t",
                    source_kind=txn_kind,
                    description_norm="ДИВИДЕНДЫ ЗА 2024",
                    direction="expense",
                )
            ],
        ),
        ENGINE_VERSION,
    )
    assert bool(response.proposals) is fires


# ---------------------------------------------------------------------------
# Task 6.8 -- layer order, stated directly rather than only via the fixture
# ---------------------------------------------------------------------------


def test_memory_outranks_a_code_which_outranks_text() -> None:
    """One transaction that all three layers answer, three different answers.

    The rules are numbered so that the text rule comes first: if the engine
    sorted by priority across layers rather than within them, it would win,
    and it must not.
    """
    text = classifier_pb2.Rule(
        priority=1,
        category_code="0404",
        scope="country:BY",
        source_kind="bank",
        all=[
            classifier_pb2.Condition(
                field="description", op="contains_all", value="ЗАРАБОТНАЯ ПЛАТА"
            )
        ],
    )
    code = classifier_pb2.Rule(
        priority=50,
        category_code="0403",
        scope="country:KZ",
        source_kind="bank",
        all=[
            classifier_pb2.Condition(field="regulated_code", op="eq", value="332")
        ],
    )
    txn = classifier_pb2.TxnForClassify(
        transaction_id="t",
        source_kind="bank",
        description_norm="ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ",
        counterparty_key="tax:120970530",
        direction="expense",
        regulated_code="332",
    )

    def answer(**extra: object) -> classifier_pb2.Proposal:
        response = engine.classify_batch(
            request(rules=[text, code], txns=[txn], **extra), ENGINE_VERSION
        )
        assert len(response.proposals) == 1
        return response.proposals[0]

    text_only = engine.classify_batch(
        request(rules=[text], txns=[txn]), ENGINE_VERSION
    ).proposals[0]
    assert (text_only.engine_layer, text_only.category_code) == ("L1", "0404")

    with_code = answer()
    assert (with_code.engine_layer, with_code.category_code) == ("L0.5", "0403")

    with_memory = answer(
        vendors=[
            classifier_pb2.VendorMemory(
                key="tax:120970530", category_code="09", display_name="x"
            )
        ]
    )
    assert (with_memory.engine_layer, with_memory.category_code) == ("L0", "09")
    assert with_memory.evidence == "tax_id"


# ---------------------------------------------------------------------------
# Task 6.9 -- priority within a layer
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("description", "category"),
    [
        # The longer phrase is seeded at priority 4 and the bare one at 32, so
        # a payment that carries both lands on dividends rather than on
        # payroll tax.
        ("ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ", "09"),
        ("ПОДОХОДНЫЙ НАЛОГ ЗА МАЙ", "0403"),
        # ОТЧИСЛЕНИЯ В ФСЗН is priority 30 and ОТПУСКНЫЕ is 39.
        ("ОТЧИСЛЕНИЯ В ФСЗН ЗА ОТПУСКНЫЕ", "0403"),
        ("ОТПУСКНЫЕ ЗА ИЮНЬ", "0404"),
    ],
)
def test_the_more_specific_rule_wins(description: str, category: str) -> None:
    response = engine.classify_batch(
        request(
            txns=[
                classifier_pb2.TxnForClassify(
                    transaction_id="t",
                    source_kind="bank",
                    description_norm=description,
                    direction="expense",
                )
            ]
        ),
        ENGINE_VERSION,
    )
    assert [p.category_code for p in response.proposals] == [category]


# ---------------------------------------------------------------------------
# Tasks 5.3 and 5.4 -- what the engine refuses
# ---------------------------------------------------------------------------


def test_an_unknown_normalize_version_is_refused() -> None:
    with pytest.raises(engine.RequestRejectedError) as raised:
        engine.classify_batch(request(normalize_version="v2", txns=[]), ENGINE_VERSION)
    assert raised.value.code == "unsupported_normalize_version"


def test_a_rule_naming_an_absent_category_is_refused() -> None:
    """Core's identifiers are not taken on trust.

    The engine has no database and cannot look a code up, so a code it was not
    given is one it must not put in a proposal -- the report would have nothing
    to render it against.
    """
    with pytest.raises(engine.RequestRejectedError) as raised:
        engine.classify_batch(
            request(
                categories=[classifier_pb2.Category(code="09", name="Dividends")],
                rules=[
                    classifier_pb2.Rule(
                        priority=1,
                        category_code="not-a-code",
                        scope="org",
                        all=[
                            classifier_pb2.Condition(
                                field="direction", op="eq", value="expense"
                            )
                        ],
                    )
                ],
                txns=[],
            ),
            ENGINE_VERSION,
        )
    assert raised.value.code == "category_not_in_request"


def test_vendor_memory_naming_an_absent_category_is_refused() -> None:
    with pytest.raises(engine.RequestRejectedError) as raised:
        engine.classify_batch(
            request(
                categories=[classifier_pb2.Category(code="09", name="Dividends")],
                rules=[],
                vendors=[
                    classifier_pb2.VendorMemory(
                        key="tax:1", category_code="not-a-code", display_name="x"
                    )
                ],
                txns=[],
            ),
            ENGINE_VERSION,
        )
    assert raised.value.code == "category_not_in_request"


@pytest.mark.parametrize(
    ("field", "op", "code"),
    [
        ("description", "eq", "unsupported_operator"),
        ("direction", "contains_all", "unsupported_operator"),
        ("amount", "eq", "unsupported_operator"),
        ("colour", "eq", "unsupported_field"),
    ],
)
def test_a_matcher_this_engine_cannot_evaluate_fails_the_batch(
    field: str, op: str, code: str
) -> None:
    """Loud, not false.

    Returning False would let a newer core's rules quietly stop firing against
    an older classifier, and a coverage number that drops for an invisible
    reason is worse than one that does not arrive.
    """
    with pytest.raises(engine.RequestRejectedError) as raised:
        engine.classify_batch(
            request(
                rules=[
                    classifier_pb2.Rule(
                        priority=1,
                        category_code="09",
                        scope="org",
                        all=[
                            classifier_pb2.Condition(field=field, op=op, value="x")
                        ],
                    )
                ],
                txns=[
                    classifier_pb2.TxnForClassify(
                        transaction_id="t", source_kind="bank", direction="expense"
                    )
                ],
            ),
            ENGINE_VERSION,
        )
    assert raised.value.code == code


def test_a_proposal_below_the_threshold_is_not_returned() -> None:
    """A machine never posts without an answer it is sure of."""
    txn = classifier_pb2.TxnForClassify(
        transaction_id="t",
        source_kind="bank",
        description_norm="ЗАРАБОТНАЯ ПЛАТА ЗА МАЙ",
        direction="expense",
    )
    assert engine.classify_batch(request(txns=[txn]), ENGINE_VERSION).proposals
    above_every_layer = engine.classify_batch(
        request(txns=[txn], threshold=0.999), ENGINE_VERSION
    )
    assert list(above_every_layer.proposals) == []
