# `eval/` — the classification taxonomy, the rule templates, and their measurement

This closes **D-1**, the classification category list, which
`docs/IMPLEMENTATION_PLAN.md` §7 marks as blocking changes 3.1, 3.2 and 4.1.

It is not a service and it is not on the request path. It is an offline
toolkit: it turns the founder's source files into a taxonomy and a set of rules,
and then measures those rules against real bank statements. Changes 3.1 and 3.2
consume the output; nothing at runtime imports from here.

## Run it

```sh
uv run --with openpyxl --with xlrd python eval/emit.py      # regenerate out/
uv run --with openpyxl --with xlrd python eval/run_eval.py  # measure it
```

Both read `../docCl` (override with `VEKST_DOCCL`). That folder holds a
customer's own exports and their accountant's categorisation, so it is
deliberately outside the repository and never committed. `eval/.statements/` is
an unpacked cache of it and is gitignored for the same reason.

## What is measured

Against 4,508 real transactions across three countries, 2023–2024:

| | Rules | Rows | Amount | Disagreements |
| --- | --- | --- | --- | --- |
| Belarus, Priorbank | 41 | 87.5% | 85.2% | 15 (0.69%) |
| Kazakhstan | 21 | 86.1% | **99.5%** | 4 (0.36%) |
| Poland | — | — | — | no template yet |

Not one of those 62 rules mentions the customer they were measured on. They
belong to a country and a bank, which is what makes them worth keeping: a new
Belarusian company on Priorbank is 87% classified before it configures
anything, and what remains is 54 counterparties — one review decision each,
after which vendor memory has them.

Kazakhstan reaches the same coverage with half the rules because every Kazakh
bank row carries **КНП**, a state payment-purpose code. Sixteen КНП/direction
pairs classify with no exceptions at all. `docs/ARCHITECTURE.md` §4.1 limits the
account-code layer to ledger rows and forbids it on bank rows; that restriction
is wrong for this market, and the numbers are the argument.

## Files

| | |
| --- | --- |
| `norm.py` | `normalize_description` and `counterparty_key`. Pure, versioned. |
| `sources.py` | Readers for the three statement formats and the three rule files. |
| `build.py` | Merges the taxonomy; builds the two templates. Every merge decision is a commented line. |
| `emit.py` | Writes `out/`. |
| `engine.py` | The engine. Pure function, layers L0 → L0.5 → L1. |
| `run_eval.py` | The harness. |

`out/` is committed: it is the deliverable, and regenerating it must produce no
diff.

| `out/…` | For |
| --- | --- |
| `taxonomy.csv`, `templates.csv` | A person to read and argue with |
| `categories.json`, `rules.json` | `engine.py` and `run_eval.py` |
| `seed_categories.sql`, `seed_rules.sql` | Changes 3.1 and 3.2 to apply |

## Four columns the schema does not have yet

The seeds do not fit `docs/ARCHITECTURE.md` §5.5 as written. Each header says so
in place; in short:

- `categories.scope` — levels 1–2 are ours and versioned; a leaf such as
  `IT Park - membership` belongs to one organisation. With no `org_id` on the
  table, that leaf has nowhere to live.
- `categories.is_computed` / `formula` — GM, NM, CM, IBT and NI are arithmetic
  over other lines. A transaction can never land in one.
- `categories.requires_allocation` — payroll is known, the department is not.
  КНП 332 says "wages" and cannot say whose.
- `classification_rules.org_id NULL` + `scope` — **87% of coverage comes from
  rules that belong to no customer.** A schema where every rule has an owning
  organisation cannot express them.

## What the harness measures, and what it refuses to call an error

The reference labelling is the accountant's own file — the only labelling there
is. Disagreement with it is not automatically a defect, so the report separates:

- **agreed** — same category.
- **awaiting allocation** — we said `PAYROLL to distribute`; they named a
  department. Correct at template level.
- **less specific** — our answer is an ancestor of theirs.
- **DISAGREED** — different branches. The number to watch: 19 across 4,508 rows.

The first version of this harness counted the middle two as failures and
reported 198 where there were 19. A metric that punishes caution makes you
optimise the wrong thing, so the distinction is kept in the code rather than in
someone's head.

## Known gaps

- **Poland has no template.** The parser exists — `Dane operacji` is a
  pipe-separated key:value structure carrying counterparty name, account and
  card merchant — but the rules are not built.
- **Seven rules never fire** on this customer's two years. Candidates for
  deletion, but only after a second customer says the same.
- **`БИКОНСАЛТ` and `BICONSULT` do not merge.** The same group entity, Cyrillic
  in Belarus and Latin in Kazakhstan: 15 rows in the Kazakh residue and 21 in
  the Polish one. Transliteration would join them and would also join companies
  that are not the same; an alias table filled from the review queue is the
  safer answer.
- **`Dev Services > Salary`** — three rules put salary under Services. Either a
  contractor billed as a service, or a mistake. Unresolved on purpose.
