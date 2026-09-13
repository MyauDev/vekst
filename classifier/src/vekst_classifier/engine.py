"""The classification engine.

A pure function of its inputs: no database handle, no clock, no globals, no
network. The same request always produces the same response. That is not a
style preference -- `docs/ARCHITECTURE.md` §2.3 requires it, because it is what
lets a March report reproduce in June, and what makes this service safe to
call twice when a job retries.

Ported from `eval/engine.py`, which stays the reference: `eval/run_eval.py`
measures that code against the founder's own statements, so the accuracy
numbers in the design describe it. The two must not drift, which is what
`tests/test_engine_fixtures.py` is for.

Layer order, and why it is this order:

  L0    the organisation's own vendor memory. Exact, earned from a human
        decision in its own review queue, and free. Nothing outranks it.
  L0.5  a regulated code carried by the source -- КНП in Kazakhstan, `Typ
        operacji` at PKO BP. Assigned by somebody other than the payer, which
        is what makes it stronger evidence than the payer's own free text.
  L1    rules over text: a country's payment vocabulary, a bank's own wording,
        and whatever the organisation added itself.

The layers are tried in that order regardless of priority, and priority orders
rules *within* a layer. Sorting the two layers together by priority alone would
make a Belarusian phrase rule outrank a Kazakh КНП simply because it was
numbered first -- and both statements are written in Russian, so that is not a
hypothetical.

Below the threshold there is no answer and the row goes to a human. A machine
never posts without one.
"""

from collections.abc import Iterable, Sequence
from dataclasses import dataclass

from vekst.internal.v1 import classifier_pb2
from vekst.type.v1 import money_pb2

# The normalisation this engine's matching assumes. Both the rule's stored
# text and the transaction's arrive already normalised -- core does it, in
# core/internal/normalize -- so this is an assertion about what it was given,
# never something applied here.
SUPPORTED_NORMALIZE_VERSIONS = frozenset({"v1"})

# Default from design D-10. A request that leaves threshold at zero gets this
# rather than "accept everything", because proto3 cannot tell an unset double
# from a deliberate 0.0 and the safe reading of the ambiguity is the stricter
# one.
DEFAULT_THRESHOLD = 0.80

LAYER_VENDOR = "L0"
LAYER_REGULATED = "L0.5"
LAYER_RULE = "L1"

# A regulated code is exact and was not written by the payer; a text match is
# a phrase that survived normalisation. Both sit above the auto-accept
# threshold, and the gap between them is what orders a review queue.
CONFIDENCE = {LAYER_VENDOR: 1.00, LAYER_REGULATED: 0.99, LAYER_RULE: 0.95}

FIELD_REGULATED_CODE = "regulated_code"

# A counterparty key carries its tier in its prefix, and the tier is what a
# reviewer needs: a regulator-issued identifier and a name that survived
# normalisation are not equally good reasons to have remembered something.
# Spelled out rather than taken from the prefix, so that the evidence string
# stays the vocabulary core uses (normalize.Tier) rather than an abbreviation
# that happens to be in the key.
TIER_BY_PREFIX = {"tax": "tax_id", "name": "name", "acct": "account"}


class RequestRejectedError(Exception):
    """A request this engine will not answer.

    Carries a code, not a sentence: the backend returns codes and the client
    translates them (CLAUDE.md, Conventions). `detail` is for the server log
    and for a developer reading a failed job, never for an end user.
    """

    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


@dataclass(frozen=True)
class Answer:
    """What a layer concluded, before the threshold is applied."""

    category_code: str
    layer: str
    evidence: str
    matched_rule_priority: int = 0

    @property
    def confidence(self) -> float:
        return CONFIDENCE[self.layer]


def _money_comparable(a: money_pb2.Money, b: money_pb2.Money) -> bool:
    """Two amounts compare only when they are the same currency.

    No conversion happens here and none should: an FX rate has a date, the
    engine has no clock, and a rule written as "over 1000 EUR" says nothing
    about a yen amount. The row falls through to the next rule instead.
    """
    return bool(a.currency_code) and a.currency_code == b.currency_code


def _condition_holds(
    txn: classifier_pb2.TxnForClassify, c: classifier_pb2.Condition
) -> bool:
    field, op, want = c.field, c.op, c.value

    if field == "description":
        # `contains_all` is an AND over ';'-separated parts: confirmed against
        # 40 of the 46 multi-part rules in the founder's files, and the six
        # exceptions are rules whose second half no longer appears in any
        # statement. Both sides are already normalised, so a bank prefix on
        # one and not the other stops mattering.
        if op != "contains_all":
            raise RequestRejectedError(
                "unsupported_operator",
                f"description supports contains_all, not {op!r}",
            )
        parts = [p.strip() for p in want.split(";") if p.strip()]
        return all(p in txn.description_norm for p in parts)

    if field == "direction":
        _require_eq(field, op)
        return txn.direction.lower() == want.lower()

    if field == FIELD_REGULATED_CODE:
        _require_eq(field, op)
        return txn.regulated_code == want

    if field == "counterparty_key":
        _require_eq(field, op)
        return txn.counterparty_key == want

    if field == "account":
        _require_eq(field, op)
        return txn.account_id == want

    if field == "amount":
        if op not in {"gte", "lte"}:
            raise RequestRejectedError(
                "unsupported_operator", f"amount supports gte and lte, not {op!r}"
            )
        if not _money_comparable(c.amount_value, txn.amount):
            return False
        if op == "gte":
            return txn.amount.minor_units >= c.amount_value.minor_units
        return txn.amount.minor_units <= c.amount_value.minor_units

    raise RequestRejectedError("unsupported_field", f"no matcher for field {field!r}")


def _require_eq(field: str, op: str) -> None:
    if op != "eq":
        raise RequestRejectedError(
            "unsupported_operator", f"{field} supports eq, not {op!r}"
        )


def _rule_matches(
    txn: classifier_pb2.TxnForClassify, rule: classifier_pb2.Rule
) -> bool:
    # A rule written for a bank's wording must not fire on a ledger row. An
    # empty source_kind is a rule that did not say, and is tried on both.
    if rule.source_kind and rule.source_kind != txn.source_kind:
        return False
    if not rule.all:
        # The database forbids this; a request is not the database.
        raise RequestRejectedError(
            "empty_matcher", f"rule at priority {rule.priority} has no conditions"
        )
    return all(_condition_holds(txn, c) for c in rule.all)


def _is_regulated(rule: classifier_pb2.Rule) -> bool:
    """Whether a rule belongs to L0.5.

    Any condition on the regulated code puts it there, not merely the first
    one: a rule reading "КНП 223 and direction expense" is evidence from the
    regulator whichever order it happens to be written in.
    """
    return any(c.field == FIELD_REGULATED_CODE for c in rule.all)


def classify_one(
    txn: classifier_pb2.TxnForClassify,
    regulated: Sequence[classifier_pb2.Rule],
    textual: Sequence[classifier_pb2.Rule],
    memory: dict[str, classifier_pb2.VendorMemory],
) -> Answer | None:
    """The first layer that answers, or None to send the row to review."""
    if txn.counterparty_key and txn.counterparty_key in memory:
        remembered = memory[txn.counterparty_key]
        # The key's tier is what the organisation's own decision rested on,
        # and it is what a reviewer needs to see to judge the answer.
        prefix = txn.counterparty_key.split(":", 1)[0]
        return Answer(
            remembered.category_code, LAYER_VENDOR, TIER_BY_PREFIX.get(prefix, prefix)
        )

    for layer, rules in ((LAYER_REGULATED, regulated), (LAYER_RULE, textual)):
        for rule in rules:
            if _rule_matches(txn, rule):
                return Answer(rule.category_code, layer, rule.scope, rule.priority)
    return None


def classify_batch(
    request: classifier_pb2.ClassifyBatchRequest, engine_version: str
) -> classifier_pb2.ClassifyBatchResponse:
    """Answer a batch, or refuse it. Nothing in between.

    Refusing the whole batch rather than skipping a row is deliberate: a
    request core could not have meant is a bug in core, and a partial answer
    would land as a coverage number nobody can explain.
    """
    if request.normalize_version not in SUPPORTED_NORMALIZE_VERSIONS:
        raise RequestRejectedError(
            "unsupported_normalize_version",
            f"{request.normalize_version!r} is not one of "
            f"{sorted(SUPPORTED_NORMALIZE_VERSIONS)}",
        )

    known = {c.code for c in request.categories}
    _reject_unknown_codes(known, (r.category_code for r in request.rules), "rule")
    _reject_unknown_codes(known, (v.category_code for v in request.vendors), "vendor")

    # Partitioned once, not per transaction. The relative order inside each
    # layer is the order core sent, which is the order the query defined.
    regulated = [r for r in request.rules if _is_regulated(r)]
    textual = [r for r in request.rules if not _is_regulated(r)]
    memory = {v.key: v for v in request.vendors}

    threshold = request.threshold or DEFAULT_THRESHOLD
    proposals = []
    for txn in request.txns:
        answer = classify_one(txn, regulated, textual, memory)
        if answer is None or answer.confidence < threshold:
            continue
        proposals.append(
            classifier_pb2.Proposal(
                transaction_id=txn.transaction_id,
                category_code=answer.category_code,
                engine_layer=answer.layer,
                confidence=answer.confidence,
                evidence=answer.evidence,
                matched_rule_priority=answer.matched_rule_priority,
            )
        )

    return classifier_pb2.ClassifyBatchResponse(
        engine_version=engine_version,
        ruleset_version=request.ruleset_version,
        proposals=proposals,
    )


def _reject_unknown_codes(
    known: set[str], used: Iterable[str], what: str
) -> None:
    """Every code a proposal could name must be in the request.

    The engine holds no database and has no way to check a code it was not
    given, so "this id exists" is not something it may assume on the other
    side's word. A rule pointing at a category core did not send is core
    sending an inconsistent request, and answering it anyway would write a
    classification against a category the report cannot render.
    """
    missing = sorted({code for code in used if code not in known})
    if missing:
        raise RequestRejectedError(
            "category_not_in_request",
            f"{what} categories absent from the request: {missing}",
        )
