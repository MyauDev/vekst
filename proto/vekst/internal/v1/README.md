# `vekst.internal.v1` — the core ↔ classifier contract

**Owner: Track B. `ClassifyBatch` arrives with change 3.2.**

This package carries the internal gRPC contract between `core` and the Python
`classifier` service. It is not browser-facing and must never be routed through
the Ingress.

## Why only `Version` exists today

`ARCHITECTURE.md` §3.2 specifies the full `ClassifyBatch` request and response:
taxonomy, vendor memory, org rules, account-code maps, transactions, threshold.
None of those types exist yet — they are designed by changes 3.1
(`add-classification-taxonomy`) and 3.2 (`add-classification-engine`).

Declaring them now would freeze guesses into a contract that `buf breaking`
then defends against correction. Change 0.1 stands up the wire; deciding what
travels over it is 3.2's job.

## Rules that apply to anything added here

- **No database credentials, ever.** The classifier is a pure function over its
  inputs. `core` reads the tenant's context under RLS and passes it in the
  request. See `ARCHITECTURE.md` A-4 and §3.3.
- **Money is a `Money` message**, never a `double`. Change 0.2 adds the type.
- Any change to this package runs `buf breaking` and needs both reviewers.
