# `data/` — the seam the backend arrives through

One module per screen that reads data. Interfaces shaped like the proto messages
that will eventually carry them; fixtures behind them today.

**Money is an `int64` minor-unit string and an ISO-4217 code. Never a
JavaScript `number`** — it cannot hold `int64` exactly, and this is the file
where a wrong number would be introduced after the backend got it right.
States are codes, never sentences: turning a code into a sentence is `i18n`'s
job.

**Components import these modules and never a transport.** Connecting the real
backend replaces a module body and touches no component
(`add-web-experience` design D1). `src/testTransport.ts` is a different
mechanism for a different purpose — it stubs Health and Identity, which have
protos; these screens do not yet.

Fixtures compute: every total, subtotal and percentage derives from the rows the
table renders. A typed-in total is a fixture that can disagree with itself.
