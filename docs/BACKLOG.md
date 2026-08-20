# Vekst — Backlog

Updated 2026-08-15, after the Palm alignment. Supersedes the earlier version.

Milestones are defined in `docs/IMPLEMENTATION_PLAN.md` section 3:
**M1 Pilot** (2026-10-01) · **M2 Commercial** (2026-11-28) · **M3 Intelligence** (2027-Q1).

Nothing here is forgotten. Each item records where it sits and what must be true before it
moves. Do not pull an item forward without removing something else from the same milestone.

---

## A. Moved INTO scope by the Palm alignment

These left the backlog on 2026-08-15. Listed so the change is visible.

| Item | Now in | Note |
| --- | --- | --- |
| Multi-entity per account | M1 schema | Palm wizard Q4/Q5 score up to 12+ legal entities. `entity_id` exists from the first migration |
| Reports beyond the P&L | M2 | Sales, OPEX, Cash Flow |
| PDF export | M2 | Rendered from the same page the user sees |
| Dunning and scheduled deletion | M2 | Palm's exact rules — see D-1 below |
| Deviation highlighting | M2 | Deterministic thresholds, no AI text |
| German interface | M2 | en + ru in M1 |
| Marketing site | M2 | Astro static |
| Chat gap-filling with escalation | M2 | Palm screen 6 |
| Consultant requests (Wizard-of-Oz) | M2 | Admin queue plus a Telegram or email relay |
| Paddle billing and add-ons | M2 | Replaces Stripe; no EU entity needed |
| Regulated chart-of-accounts mapping (L0.5) | M1 | From the Palm spec, for 1C rows only |

---

## B. M3 — after the first paying customers

### B-1. AI classification layer (L4)
Method as designed in the Palm spec: embeddings narrow to 3–5 candidates, then an LLM
picks one with structured output and a confidence score.
Out of M1 and M2 because: without a rules-only baseline there is no number to compare
against, and an LLM call per line has a cost that never falls while vendor memory gets
cheaper every month.
Slots in as engine layer 4 inside the existing Python service. No schema change — the
`engine_layer`, `ruleset_version` and `engine_version` columns already exist.
Bring back when: you have a measured unmatched rate on real customer files across at
least 3 tenants.
Watch: EU data processing agreement, no-training terms, redaction of account numbers and
person names before the call, cost per line, and a hard rule that the model never writes
without human approval.

### B-2. Accounting system connectors
Xero and QuickBooks OAuth first, because Palm names them and both have real APIs.
1C stays on the file path until a customer pays for a local connector.
Each connector is 2 to 6 weeks. Do not start one until 3 customers ask for the same one.

### B-3. Mobile client
Palm plans it as the stage after web. The API is already the single entry point for both,
so this is a client project, not a backend one.

### B-4. AI comments on highlighted deviations
Text that explains an already-highlighted deviation. The rule that decides *what counts as*
a deviation stays deterministic, permanently.

### B-5. AI recommendations for corrections
The step after B-4. Do not start it before B-4 has been in front of real customers.

### B-6. Cross-client shared classification memory
The single largest accuracy gain available, because vendors repeat across companies.
It is a legal and consent decision, not a technical one, and it contradicts the "every
tenant fully isolated" position taken earlier.
**Must be decided before the first customer signs the terms of service.** Retro-active
consent is not consent. A possible middle path: a curated global vendor list that you own,
built from anonymised aggregates, opt-in per tenant.

### B-7. Remaining report templates, up to 15
Ordered by the demand data collected on screen 3.

---

## C. Not scheduled

### C-1. More file formats
- `1CClientBankExchange` (`kl_to_1c.txt`) — the CIS bank exchange standard, Windows-1251.
  Probably the highest-value next format for this market.
- CAMT.053 and MT940 — the European bank statement standards.
- SAF-T — required in Poland, Norway, Portugal, Lithuania, Romania.
- DBF — old 1C exports. OFX and QIF — long tail.

### C-2. Receipt and invoice OCR
Reads a photographed receipt or supplier invoice for vendor, date, total, VAT and line
items, then attaches it to the matching bank transaction. A bank line only says
`CARD PURCHASE *AMZN`; OCR gives the real vendor and the VAT.
Out because: it needs document storage, an OCR engine, a matching algorithm and a
correction UI. 4 to 6 weeks on its own.
Bring back when: users ask "why can I not see what this card payment was for".

### C-3. Write-back into the accounting system
Keep the product read-only for a long time. Write-back turns a reporting tool into a
system of record, and the liability changes completely.

### C-4. Budgets and forecasts
Needs its own data-entry surface: import a budget, edit per category per month, versioned.
A milestone, not a report variant.

### C-5. Custom report builder
The "Power BI" trap. Fixed templates plus filters plus saved views cover the first 20
customers. Bring back when 5 customers ask for the same report you refuse to build.

### C-6. Public read-only share link
Conflicts with the security position, and inviting the accountant as a `viewer` covers the
same need. If it ever returns: signed token, hard expiry, optional password, per-report
scope, revocation list, rate limit, and an audit-log entry.

### C-7. Scheduled email delivery of reports
Cheap once export and jobs exist. Bring back when exports are used weekly.

### C-8. Column-store analytics (ClickHouse or DuckDB)
Postgres with pre-aggregated monthly summaries handles tens of millions of rows.
Bring back when a real report query passes 2 seconds on a real tenant and the summary
table cannot fix it.

### C-9. SOC 2 and ISO 27001
Prepare now at zero cost: keep the audit log, keep access reviews, keep a change log, keep
dependency scanning in CI, and write down the backup and restore procedure. That is most
of the evidence an auditor asks for.

### C-10. Roles beyond the four
Custom roles, per-category permissions, multi-step approval. M1 and M2 have `owner`,
`admin`, `approver`, `viewer` and a single approver.

### C-11. More languages
en + ru in M1, de in M2. Later candidates: Polish, Ukrainian, Kazakh.

---

## D. Rules to carry forward when these are built

### D-1. Dunning, exactly as the Palm spec states it
- Subscription not renewed → report access closes, **data and mappings are kept**.
- An automatic email: work paused for non-payment, data kept and available for 3 months.
- 3 automatic reminders during that period.
- No response → irreversible deletion at 3 months.
- `scheduled_deletion_at` is written once, at the moment of pause, and is **never
  recomputed dynamically**.

### D-2. Consultant pricing, as stated
$30 per report at first setup — 3 hours × $10, a single charge, no hourly tracking.
Report-rework hours default to the same $10/hour unless the founder decides otherwise.
A new report outside the 15 templates is quoted individually, never at a fixed price.

---

## E. Rejected, with reasons

| Item | Reason |
| --- | --- |
| A general BI canvas with a formula language | No moat. Microsoft gives it away. The moat is classification accuracy and ingest |
| Float for money, in Go or in Python | Rounding errors no customer forgives. Pandas makes this the default path, so it gets a test |
| Schema-per-tenant isolation | Migration cost grows with every customer |
| Giving the Python classifier database credentials | Breaks single-mechanism tenant isolation and breaks reproducibility |
| An LLM as the first classification layer | Cost per line never falls, and there is no baseline to measure against |
| Merging ledger and bank data without a matching layer | Counts an invoice and its payment twice |
| Automatic posting to a ledger without human approval | The customer files the return and carries the liability |
| Stripe as the payment processor | Does not support a business located in Ukraine or Kazakhstan. Paddle does |
| A message broker between `core` and `classifier` | The call already runs inside a River job with retries |
