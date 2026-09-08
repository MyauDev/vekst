"""The classification engine.

A pure function of its inputs: no database handle, no clock, no globals, no
network. The same request always produces the same response. That is not a
style preference — `docs/ARCHITECTURE.md` §2.3 requires it, because it is what
lets a March report reproduce in June and what makes a port to another process
mechanical rather than a rewrite.

This is the reference implementation. `run_eval.py` measures *this* code, so a
number in the report describes what would actually run, not a separate
approximation of it.

Layer order, and why it is this order:

  L0    the organisation's own vendor memory. Exact, earned from a human
        decision, and free. Nothing outranks it.
  L0.5  a regulated code carried by the source — КНП in Kazakhstan. Issued by
        the state, so it beats any guess made from text. ARCHITECTURE.md limits
        this layer to ledger rows; a Kazakh *bank* row carries one, and the
        measurements say to trust it.
  L1    rules: a country's payment vocabulary, a bank's own wording, and
        whatever the organisation added itself.

Below the threshold there is no answer, and the row goes to a human. A machine
never posts without one.
"""

from norm import counterparty_key, normalize_description

ENGINE_VERSION = "l1-v1"


def _condition_holds(txn: dict, c: dict) -> bool:
    field, op, want = c["field"], c["op"], c["value"]

    if field == "description":
        # Both sides are normalised, so a bank prefix on one and not the other
        # stops mattering. `contains_all` is an AND over ';'-separated parts:
        # confirmed against 40 of the 46 multi-part rules in the source files,
        # and the six exceptions are rules whose second half no longer appears
        # in any statement.
        haystack = normalize_description(txn.get("purp", ""))
        parts = [p.strip() for p in want.split(";") if p.strip()]
        if op == "contains_all":
            return all(p in haystack for p in parts)
        return want in haystack

    if field == "direction":
        return txn.get("dir", "").lower() == str(want).lower()

    if field == "knp":
        return (txn.get("knp") or "") == want

    if field == "operation_type":
        # PKO BP labels every row with its own operation type. Weaker evidence
        # than a regulated code — the bank chose the vocabulary — but it needs
        # no text at all, and it is exact.
        return (txn.get("typ") or "") == want

    if field == "counterparty_account":
        # Matched against the parsed field, never against the packed
        # `Dane operacji` blob. The blob also carries `Rachunek:` — the
        # customer's *own* account — and a substring search over it confuses
        # the two, which silently turned 22 social-insurance payments into
        # currency conversions.
        return normalize_description(want) in normalize_description(txn.get("acct", ""))

    if field == "counterparty_key":
        return (
            counterparty_key(
                txn.get("cp", ""), txn.get("tax", ""), txn.get("acct", "")
            )[0]
            == want
        )

    return False


def classify(txn: dict, rules: list, memory: dict | None = None) -> dict | None:
    """Return the first layer that answers, or None to send the row to review.

    `rules` must already be ordered by priority: the caller owns the order, so
    the engine has no opinion the fixtures cannot express. `memory` maps a
    counterparty key to a category code and is what the review queue fills.
    """
    if memory:
        key, tier = counterparty_key(
            txn.get("cp", ""), txn.get("tax", ""), txn.get("acct", "")
        )
        if key in memory:
            return {
                "category": memory[key],
                "layer": "L0",
                "rule": key,
                "confidence": 1.00,
                "evidence": tier,
            }

    for r in rules:
        if all(_condition_holds(txn, c) for c in r["matcher"]["all"]):
            by_code = r["matcher"]["all"][0]["field"] == "knp"
            return {
                "category": r["category_code"],
                "layer": "L0.5" if by_code else "L1",
                "rule": r["priority"],
                # A regulated code is stronger evidence than a text match,
                # and both sit above the 0.80 auto-accept threshold.
                "confidence": 0.99 if by_code else 0.95,
                "evidence": r["scope"],
            }
    return None
