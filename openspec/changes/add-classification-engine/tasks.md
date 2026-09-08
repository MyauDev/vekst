**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2.5 person-days** to change 3.2.
These tasks total **≈ 26 hours ≈ 3.3 person-days**. The overrun is the Go port of
normalisation and its conformance test — about 5 hours that the plan never budgeted anywhere,
because when it was written normalisation was not known to be a separate concern. It is not
optional: `dedup_hash` depends on it, so Track A needs it whether or not this change lands.

If it must fit 2.5 days, move the port and the conformance test into change 2.5, where the
column lives. Do not drop them.

**Ordering.** Apply after 3.1 (`categories` must exist for the foreign keys) and after 1.1.
Task 0.1 is a dependency Track A owns and should hear before its own migration is written.

**Ownership.** Track B, except 0.1 which is a conversation with Track A.
`/proto` needs both reviewers.

## 0. Raise with Track A and with the architecture doc

- [ ] 0.1 Tell Track A that change 2.5 needs `description_norm`, `normalize_version` and `regulated_code` on `transactions` (design §D2), before its migration is written
- [ ] 0.2 Amend `ARCHITECTURE.md` §4.1: the account-code layer is not ledger-only. Record the measurement that overturns it (design §D3)

## 1. Migration — Track B

- [ ] 1.1 Migration 006 up: `classification_rules` with its check constraint and the unique priority index
- [ ] 1.2 Migration 006: `vendors`, with `key_version` and the unique key per organisation
- [ ] 1.3 Seed the 71 template rules from `eval/out/seed_rules.sql`, resolving `category_code` to `category_id`, **before** RLS is enabled — same ordering as 3.1
- [ ] 1.4 Policies: split read/write on `classification_rules`, ordinary tenant policy on `vendors`; enable and `FORCE` both
- [ ] 1.5 Migration 006 down, and `up → down → up` against a scratch database

## 2. Generated queries — Track B

- [ ] 2.1 `core/internal/db/query/classify.sql`: effective rules for an organisation, ordered by priority; vendor memory for an organisation
- [ ] 2.2 Run `make gen`; confirm the codegen drift job stays green

## 3. Proto — Track B, both reviewers

- [ ] 3.1 Add `ClassifyBatch` and its messages to `proto/vekst/internal/v1/classifier.proto` per the design, including `reserved 20 to 39`
- [ ] 3.2 Money fields use `vekst.type.v1.Money`; no `double` anywhere except `confidence` and `threshold`
- [ ] 3.3 Run `buf lint` and `buf breaking`; adding an RPC and messages is additive and must pass
- [ ] 3.4 Replace `classifier/tests/test_service.py::test_classify_batch_is_not_declared` with a test asserting the contract now declares exactly `Version` and `ClassifyBatch` — the stop-cock moves, it does not disappear
- [ ] 3.5 Run `make gen`; commit the regenerated Go, Python and TypeScript output

## 4. Normalisation in Go — Track B

- [ ] 4.1 Port `normalize_description` from `eval/norm.py` to `core/internal/normalize`
- [ ] 4.2 Port `counterparty_key`, including the tax-identifier tier and the address trimming
- [ ] 4.3 **Conformance test:** run the Go implementation over the committed fixtures and fail on any difference from the Python reference
- [ ] 4.4 Export `NormalizeVersion` and assert it is recorded wherever a normalised value is stored

## 5. The engine — Track B, Python

- [ ] 5.1 Port `eval/engine.py` into `classifier/src/vekst_classifier/engine.py`: layers L0 → L0.5 → L1, first match wins
- [ ] 5.2 Field switch for `description`, `counterparty_key`, `regulated_code`, `direction`, `amount`, `account` — matched against the named field alone, never a concatenation
- [ ] 5.3 Reject a request whose `normalize_version` the engine does not implement, with a specific error code
- [ ] 5.4 Reject a proposal naming a `category_code` absent from the request, rather than trusting the other side's identifiers
- [ ] 5.5 Wire `ClassifyBatch` into `ClassifierService`; keep the servicer free of state, clocks and globals
- [ ] 5.6 Go: add `Classify` to the `Classifier` interface and to `GRPCClient`; `Unavailable` returns the same error shape as today

## 6. Tests — Track B

- [ ] 6.1 **Fixture test, Python:** replay `eval/out/rules.json` and assert the engine reproduces the harness's per-country coverage and accuracy exactly
- [ ] 6.2 **Determinism:** the same request twice returns byte-identical responses
- [ ] 6.3 **No-float money:** a reflection test failing if any money field in the generated Python types is floating point
- [ ] 6.4 **Non-base-currency:** an amount-range rule fires correctly on JPY (exponent 0) and KWD (exponent 3), and never compares two different currency codes
- [ ] 6.5 **Cross-tenant isolation, `vendors`:** organisation A cannot read or write B's memory
- [ ] 6.6 **Cross-tenant isolation, `classification_rules`:** A sees every template rule and none of B's own; A cannot write a template rule
- [ ] 6.7 **Negative:** a ledger-only rule does not fire on a bank row, and a bank-only rule does not fire on a ledger row
- [ ] 6.8 **Layer order:** vendor memory outranks a regulated code, which outranks a text rule, on a transaction that all three match
- [ ] 6.9 **Priority:** on a row matching both, `ПОДОХОДНЫЙ НАЛОГ;ИЗ ДИВИДЕНДОВ` wins over `ПОДОХОДНЫЙ НАЛОГ`, and `ОТЧИСЛЕНИЯ В ФСЗН` wins over `ОТПУСКНЫЕ`
- [ ] 6.10 **Go client:** `Classify` honours its deadline and surfaces an unreachable classifier as a retryable error, as `Version` already does

## 7. Close

- [ ] 7.1 Add `/core/internal/normalize/` to `CODEOWNERS` under Track B
- [ ] 7.2 Update `docs/IMPLEMENTATION_PLAN.md` §3 with the actual cost, and §7 with the amended §4.1 decision
- [ ] 7.3 Update the capability spec and run the full suite
