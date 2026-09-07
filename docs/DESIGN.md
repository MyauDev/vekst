# Vekst — UI design direction v1

Date: 2026-09-06 · Applies from change 5.1 `add-web-app-shell` onward.
Companion documents: `WORKFLOW.md` §5, `ARCHITECTURE.md`, `openspec/config.yaml`.

> This document decides how the product looks and why. It is a constraint on
> future work, in the same sense as the invariants in `CLAUDE.md`. Where it
> disagrees with a component library, this document wins.

---

## 0. What this document decides

| Decision | Section |
| --- | --- |
| Two visual registers, not one | 1 |
| What "minimal" is allowed to remove | 2 |
| The semantic token set for Tailwind v4 | 3 |
| Typography, numerals and money rendering | 4 |
| The density rules for product screens | 5 |
| What colour is allowed to mean | 6 |
| The state vocabulary, in two languages | 7 |
| The four screen skeletons for the Demo | 8 |
| Keyboard and accessibility floor | 9 |

It does not decide: chart colour ramps (change 5.2 and `WORKFLOW.md` §5.3),
dark-mode values (Product), or component-by-component markup.

---

## 1. Two registers

**The application is the minimal one.** The landing page is one page with one
promise and one call to action; it is not the design problem. The design
problem is a minimal application that still shows a twelve-column P&L.

| | Register A — Landing | Register B — App |
| --- | --- | --- |
| Where | One landing page. `/site` later | Everything behind sign-in |
| Job | One promise, one call to action | Read many numbers without error |
| Type scale | Up to 48 px | Caps at 24 px |
| Spacing base | 8 px, sections at 64–96 px | 4 px, sections at 16–24 px |
| Elements per view | Few | Many, ordered |
| Motion | Allowed, restrained | Overlays only |
| Colour | One accent, large areas | Neutral chrome, colour for meaning only |

Both registers use the **same tokens** in section 3. They differ in the spacing
scale, the type scale and the element count. Nothing else.

### 1.1 What minimal means at high density

The minimal look and a dense table do not conflict, but only under one rule:

> **Buy white space by removing chrome, not by adding padding.**

In practice, in Register B:

| Do | Instead of |
| --- | --- |
| One 1 px hairline between groups | A bordered card per section |
| Weight and colour for hierarchy | Larger type |
| One accent, one primary action per view | A coloured button per row |
| Labels only where the column header is not enough | An icon beside every label |
| A flat table on the page surface | A table inside a card inside a panel |
| 32 px rows with 8 px cell padding | 48 px rows with 16 px padding |

Every row of that table removes something and keeps the row count. A minimal
design that raises padding instead lowers the rows per screen, and an
accountant who compares twelve periods then scrolls to compare. That is the one
failure mode to watch for.

**Unlumen** (`https://ui.unlumen.com/`) remains the design reference and the
source of component primitives. Its defaults are Register A. Register B
overrides the spacing and the type scale when a primitive is used behind
sign-in — that override is the only change this document makes to it.

---

## 2. The rule that resolves conflicts

**Minimal means less decoration. It never means less information.**

May be removed: shadows used for hierarchy, gradients, card borders inside
cards, icon-plus-label where the label alone is clear, decorative dividers,
hover-only reveals, animated numbers, section headers that repeat the nav.

May never be removed:

- The basis label on any report line (`cash-basis` or `accrual`).
- The reconciliation strip.
- The reason a figure is blocked.
- The engine layer and the confidence on a classified row.
- The original file line number in a validation error.
- The counts in an import summary — rows imported, duplicates skipped,
  internal transfers found, matches proposed.

Every item in the second list is what makes a number trustworthy. Each one is
also the first thing a minimal redesign deletes. That is why the list exists.

---

## 3. Tokens

Tailwind v4 is CSS-first. The token layer lives in `web/src/index.css` under
`@theme`. Names are **semantic**, never literal. `text-slate-900` in a
component is a defect: it makes the Product-milestone dark mode a rewrite
instead of a second block of values.

```css
@import "tailwindcss";

@theme {
  /* Surfaces */
  --color-surface:         oklch(99%  0.002 95);
  --color-surface-raised:  oklch(100% 0     0);
  --color-surface-sunken:  oklch(97%  0.003 95);
  --color-overlay:         oklch(100% 0     0);

  /* Text */
  --color-text:            oklch(24%  0.008 95);
  --color-text-muted:      oklch(48%  0.008 95);
  --color-text-subtle:     oklch(56%  0.006 95);
  --color-text-inverse:    oklch(99%  0.002 95);

  /* Lines */
  --color-border:          oklch(91%  0.004 95);
  --color-border-strong:   oklch(84%  0.005 95);

  /* Action */
  --color-accent:          oklch(48%  0.130 250);
  --color-accent-hover:    oklch(42%  0.130 250);
  --color-accent-text:     oklch(99%  0.002 95);
  --color-focus:           oklch(55%  0.160 250);

  /* State — meaning, not decoration */
  --color-ok:              oklch(52%  0.110 150);
  --color-ok-surface:      oklch(96%  0.020 150);
  --color-warn:            oklch(54%  0.120  75);
  --color-warn-surface:    oklch(96%  0.030  75);
  --color-danger:          oklch(52%  0.170  27);
  --color-danger-surface:  oklch(96%  0.025  27);

  /* Figures. Deliberately not ok/danger. See §6. */
  --color-figure:          var(--color-text);
  --color-figure-negative: var(--color-text);
  --color-figure-blocked:  var(--color-text-subtle);

  /* Type */
  --font-sans: "Inter Variable", "Inter", system-ui, sans-serif;
  --font-mono: "JetBrains Mono", ui-monospace, monospace;

  --text-2xs: 0.6875rem; /* 11px — table meta only */
  --text-xs:  0.75rem;   /* 12px */
  --text-sm:  0.8125rem; /* 13px — table numerals */
  --text-base:0.875rem;  /* 14px — product body */
  --text-md:  1rem;      /* 16px */
  --text-lg:  1.25rem;   /* 20px */
  --text-xl:  1.5rem;    /* 24px — product maximum */

  /* Radius */
  --radius-control: 3px;
  --radius-panel:   6px;

  /* Motion */
  --ease-panel: 120ms cubic-bezier(0.2, 0, 0, 1);
}
```

Dark mode is a Product item budgeted at 0.5 days. That price holds **only** if
every component uses these names. Enforce it with a lint rule or a test that
greps `web/src` for literal Tailwind colour classes.

---

## 4. Typography, numerals and money

**One family: Inter.** The reason is Cyrillic. The Demo ships `en` and `ru`.
A font without a complete Cyrillic set fails half the interface, and the
failure appears only in the second language, which nobody tests first.

Russian strings run roughly 10–15 percent longer than English. No layout may
depend on a fixed label width, and no text may live inside an icon. [Likely]

**Every numeric cell uses `font-variant-numeric: tabular-nums`.** Columns of
figures must align by digit. This is not a preference.

Money rendering rules:

| Rule | Reason |
| --- | --- |
| Right-aligned, tabular numerals | Digit alignment across periods |
| Formatted from the `int64` minor units and the ISO code, never from a JS `number` | The wire invariant. `web/src/money.test.ts` guards the generated field type |
| Grouping and decimal marks from the locale (`Intl.NumberFormat`) | `ru` uses a space group and a comma decimal |
| Negative shown with a minus sign, **not** parentheses, **not** red | Two locales; parentheses are an English accounting convention. Red is reserved for failure — see §6 |
| The currency code appears once in the column header, not in each cell | Repetition costs a column of width |
| A true zero renders `0.00`. A line with no data renders `—` | An accountant must tell "nothing happened" from "no data loaded" |

---

## 5. Density — Register B, the app

- **Spacing scale:** 2, 4, 6, 8, 12, 16, 24, 32. Nothing above 32 inside the app.
- **Row height:** 32 px in tables, 36 px in the review queue (it takes keyboard
  focus and needs a larger target).
- **Borders instead of shadows.** Hierarchy comes from 1 px `--color-border`
  hairlines and from surface steps. Shadows are for true overlays only: the
  drill-down panel, a menu, a dialog.
- **No card inside a card.** One border level per view.
- **Sticky where a table scrolls.** The P&L has a period per column. Freeze the
  category column and the header row. Horizontal scroll is expected, not a bug.
- **No zebra striping.** Hairlines between section groups instead. Zebra fights
  the state colours in §7.
- **Motion:** the drill-down panel at 120 ms. Everything else at zero.
  **Never animate a figure.** A number that counts up looks like it is still
  computing, and this product's whole claim is that the number is final.

---

## 6. What colour is allowed to mean

Chrome is neutral. Colour carries exactly three jobs:

1. **The primary action** — one accent, one button per view.
2. **State** — ok, warning, danger, per §7.
3. **Category identity in charts** — see `WORKFLOW.md` §5.3.

Two rules that are easy to get wrong:

- **A negative figure is not an error.** An expense line is negative every
  month. Painting it red trains the user to ignore red, and red is what marks a
  rejected batch. `--color-figure-negative` therefore resolves to the normal
  text colour in the Demo.
- **The accent is blue, not green.** Green is spent on `ok`. An accent that
  matches a state colour makes "this is a button" and "this passed" look the
  same. `Vekst` means growth; the growth is in the numbers, not the chrome.

Deviation highlighting (`WORKFLOW.md` §5.2) arrives at Commercial. It uses the
warning surface, and it always shows the rule that fired. It never uses a
colour the reader has to interpret alone.

---

## 7. State vocabulary

Three axes. Each state has one word, one token pair and one icon, used
identically on every screen. **State is never communicated by colour alone** —
a word or an icon always accompanies it.

| Axis | State | Token | en | ru (draft — confirm) |
| --- | --- | --- | --- | --- |
| Import batch | pending | subtle | Pending | Ожидает |
| | parsing | info | Parsing | Обработка |
| | rejected | danger | Rejected | Отклонён |
| | imported | ok | Imported | Импортирован |
| Report readiness | ready | ok | Ready | Готов |
| | partial | warn | Partial | Частично |
| | blocked | danger | Blocked | Заблокирован |
| Row classification | classified | subtle | Classified | Классифицировано |
| | needs review | warn | Needs review | На проверку |
| | blocked | danger | Blocked | Заблокировано |

`partial` and `blocked` always carry their reason in the same view. "Blocked"
without the missing input named is a dead end, and the readiness check exists
precisely to name it.

The Russian column is a draft written by a non-native writer. Confirm it before
change 5.1b freezes the message catalog.

---

## 8. Screen skeletons — the Demo

**App shell.** Left rail: Imports · Review · Reports. Top bar: organisation,
**entity**, period range, language. The entity selector ships even though v1
creates one entity per organisation. The schema carries `entity_id` from the
first migration for the same reason: the slot is cheap now and a retro-fit is
not.

**Imports.** A batch list with the state from §7 and the `source_kind` tag on
every row. Upload accepts CSV and XLSX and asks for `ledger` or `bank` before
the file is read — the tag decides the accounting basis and is not optional.
The validation failure view reuses the wording in `WORKFLOW.md` verbatim; it is
close to final copy. The error list is keyed by the **original file line
number** and is downloadable.

**Review.** One counterparty group at a time, sorted by amount, then by repeat
count. The keyboard legend is always visible, never behind a help icon:
digits pick a category, `Enter` approves, `T` marks an internal transfer, `N`
marks non-P&L. A `viewer` sees the same screen read-only.

**Report — Management P&L.** Frozen category column, frozen header, one column
per period, then Total and % of revenue. The basis label sits at the top of the
table, not in a footnote. The reconciliation strip sits at the bottom: opening,
in, out, transfers, closing. Any cell opens the drill-down panel, which lists
the transactions with category, engine layer, confidence and match evidence.

---

## 9. Keyboard and accessibility floor

- **A visible focus ring on every interactive element**, 2 px `--color-focus`,
  1 px offset. The review queue is keyboard-first; a hidden focus ring makes it
  unusable.
- **No hover-only information.** Touch and keyboard users get the same content.
  A tooltip may repeat, never reveal.
- **Contrast**, measured against `--color-surface` on 2026-09-06:

  | Pair | Ratio |
  | --- | --- |
  | `text` | 15.98:1 |
  | `text-muted` | 6.35:1 |
  | `text-subtle` | 4.52:1 |
  | `accent` | 6.36:1 |
  | `accent-text` on `accent` | 6.36:1 |
  | `focus` | 4.71:1 |
  | `ok` on `ok-surface` | 4.69:1 |
  | `warn` on `warn-surface` | 4.59:1 |
  | `danger` on `danger-surface` | 5.27:1 |

  `--color-border` and `--color-border-strong` are hairlines, not the only
  indicator of any boundary, so the 3:1 rule for interface components does not
  apply to them. The outline of an input or of any focusable control uses
  `--color-text-subtle`, which passes at 4.52:1.
- **Print** must produce a readable P&L in black and white. PDF export arrives
  at Commercial and renders the same page, so the state icons in §7 are what
  survive the loss of colour.

---

## 10. What this changes in the plan

Change 5.1 is budgeted at 2 days for "Tailwind token layer … light mode only,
en + ru". Three items inside it are unbudgeted:

| Item | Why it is not free |
| --- | --- |
| An i18n library and the `ru` plumbing | Nothing is installed in `web/package.json` today |
| The error-code → sentence catalog | The invariant says the backend returns codes. The client owns every sentence |
| ECharts | Named in `ARCHITECTURE.md`, absent from `web/package.json` |

**Split the change.**

- **5.1a `add-web-app-shell`** — tokens, shell, nav, entity and period
  selectors, the state components from §7. Not blocked by anything.
- **5.1b `add-i18n-and-error-catalog`** — the i18n mechanism, then the catalog.
  The **mechanism** is not blocked. The **catalog is blocked by change 2.3**,
  because the error codes it translates do not exist until ingest validation
  defines them.

Treating 5.1 as one 2-day change hides that dependency and puts a blocked task
inside an unblocked change.

---

## 11. Rejected alternatives

| Rejected | Reason |
| --- | --- |
| Minimal read as "more padding" | It lowers the rows per screen on the P&L. Minimal here means less chrome at the same density. See §1.1 |
| Red for negative figures | Every expense line is negative. Red then means nothing. See §6 |
| Shadows for hierarchy | They cost vertical space and read as noise at 32 px row height |
| Adopting a component library's spacing and type scale as-is | Library defaults are Register A. Behind sign-in they halve the visible row count |
| Dark mode in the Demo | It is a Product item. Semantic tokens keep it at 0.5 days |
| Parentheses for negative money | An English accounting convention. The Demo ships `ru` as a first-class language |
| Accounting-green as the accent | Collides with the `ok` state |

---

## 12. Open questions

1. The Russian state words in §7 need a native check before 5.1b.
2. Register A has no owner and no change. `/site` is in the monorepo layout but
   the directory does not exist, and `WORKFLOW.md` §0 puts the landing page
   outside the Demo. Decide whether it is Product work or marketing work.
3. Inter Variable must be self-hosted or loaded from a CDN. Self-hosting adds
   about 100 kB per subset and removes a third-party request. Not decided.
4. The literal-colour lint rule in §3 has no home. It belongs in `make lint`.
