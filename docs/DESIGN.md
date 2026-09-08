# Vekst — UI design direction v1

Date: 2026-09-06 · Applies from change 5.1 `add-web-app-shell` onward.
**Amended 2026-09-08** — §3, §4, §6, §9, §11 and §12. The chrome is monochrome,
the accent is ink rather than blue, and dark mode ships rather than waiting for
Product. The reasoning is in `.design/web-app-shell/DECISIONS.md` §7–§10.
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
| The chart palette, and where hue is allowed to mean identity | 13 |
| The shapes this must not fall back into | 14 |

It does not decide: component-by-component markup.

~~Chart colour ramps~~ and ~~dark-mode values~~ were both listed here as out of scope and
both were decided on 2026-09-08 — §13 and §3 respectively. Charts were brought forward from
Commercial and dark mode from Product.

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

  /* Action. Ink, not blue — amended 2026-09-08. See §6. */
  --color-accent:          oklch(24%  0.008 95);
  --color-accent-hover:    oklch(16%  0.008 95);
  --color-accent-text:     oklch(99%  0.002 95);
  --color-focus:           oklch(24%  0.008 95);

  /* State — meaning, not decoration */
  --color-ok:              oklch(44%  0.110 150);
  --color-ok-surface:      oklch(96%  0.020 150);
  --color-warn:            oklch(47%  0.110  75);
  --color-warn-surface:    oklch(96%  0.030  75);
  --color-danger:          oklch(50%  0.170  27);
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

**These values now live in `web/src/index.css` and that file is authoritative.**
It carries two complete palettes, not one: the light values above, and a dark
set chosen rather than inverted (§9 measures both). The raw values sit on
`:root` and `[data-theme="dark"]`; `@theme inline` maps the semantic names onto
them, which is what makes a utility resolve per element instead of freezing
against `:root`.

Dark mode was a Product item budgeted at 0.5 days and now ships with the shell
(§11). That price held **only** if every component uses these names, so the
enforcement is no longer optional: `scripts/check-web-tokens.sh`, called from
`make lint`, fails the build on a literal colour class, a raw hex or oklch value
in a component, `black`/`white`, and — for Register B only — any gap above 32px.
It found 22 violations across all five production components on the day it was
written, which is the argument for having it.

---

## 4. Typography, numerals and money

**One family: Inter, self-hosted.** The reason is Cyrillic. The Demo ships
`en` and `ru`.
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
- **The accent is ink, not a hue.** *Amended 2026-09-08.* The chrome is
  monochrome: every widget is black, white or grey, and the primary action is
  carried by weight and surface rather than colour. The original rule here was
  "blue, not green", because green is spent on `ok` and an accent matching a
  state colour makes "this is a button" and "this passed" look the same.
  Removing the hue entirely satisfies that constraint absolutely, and it leaves
  **state as the only colour in the interface** — which is the strongest
  possible version of the rule this section opens with. `Vekst` means growth;
  the growth is in the numbers, not the chrome.

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
- **Contrast**, measured against each palette's own `--color-surface`,
  recomputed 2026-09-08 from the oklch values in `web/src/index.css`. Every
  pair clears WCAG AA at 4.5:1.

  | Pair | Light | Dark |
  | --- | --- | --- |
  | `text` | 15.98:1 | 15.30:1 |
  | `text-muted` | 6.35:1 | 7.58:1 |
  | `text-subtle` | 4.52:1 | 5.17:1 |
  | `accent` | 15.98:1 | 15.30:1 |
  | `accent-text` on `accent` | 15.98:1 | 15.30:1 |
  | `focus` | 15.98:1 | 10.07:1 |
  | `ok` on `ok-surface` | 6.61:1 | 6.98:1 |
  | `warn` on `warn-surface` | 6.17:1 | 7.50:1 |
  | `danger` on `danger-surface` | 5.74:1 | 4.92:1 |

  `accent` and `focus` rose from 6.36:1 and 4.71:1 because they became ink (§6).
  The three state pairs rose because their light values were deepened: the
  originals passed at 4.69, 4.59 and 5.27, which left no room for a later tweak,
  and in a monochrome interface the state colours are the only ones carrying
  meaning. Dark is not an inversion — `WORKFLOW.md` §5.3 requires chosen steps,
  and the dark ground is `oklch(18%)`, a warm near-black rather than `#000`,
  because white text on true black blooms and a P&L is read in figures.

  `--color-border` and `--color-border-strong` are hairlines, not the only
  indicator of any boundary, so the 3:1 rule for interface components does not
  apply to them. The outline of an input or of any focusable control uses
  `--color-text-subtle`, which passes in both palettes.
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
| ~~Dark mode in the Demo~~ | **Reversed 2026-09-08.** Both palettes ship with the shell. The 0.5-day price was always conditional on semantic tokens, and `scripts/check-web-tokens.sh` now enforces them, so the condition is met rather than hoped for |
| Parentheses for negative money | An English accounting convention. The Demo ships `ru` as a first-class language |
| Accounting-green as the accent | Collides with the `ok` state |
| A blue accent | Superseded 2026-09-08 by an ink accent. Any hue in the chrome competes with the three state colours, which are now the only colour in the interface. See §6 |

---

## 12. Open questions

1. The Russian state words in §7 need a native check before 5.1b.
2. Register A has no owner and no change. `/site` is in the monorepo layout but
   the directory does not exist, and `WORKFLOW.md` §0 puts the landing page
   outside the Demo. Decide whether it is Product work or marketing work.
3. ~~Inter Variable must be self-hosted or loaded from a CDN.~~ **Answered
   2026-09-08: self-hosted**, two subsets, latin and cyrillic. It matches the
   same-origin posture of design D6, needs no egress from the cluster and keeps
   the CSP simple. The `@font-face` block is in `web/src/index.css` §5, commented
   until the files are added under `web/public/fonts/`; until then the fallback
   stack renders and no layout depends on it.
4. ~~The literal-colour lint rule in §3 has no home.~~ **Answered 2026-09-08:**
   `scripts/check-web-tokens.sh`, called from `make lint`, alongside
   `check-db-entry-point.sh` and `check-identity-queries.sh`.


---

## 13. The chart palette

*Added 2026-09-08, when charts were brought forward from Commercial. §0 previously deferred
this. Values live in `web/src/index.css`; the reasoning for each is in
`openspec/changes/add-web-experience/design.md` D12.*

### 13.1 The rule that makes charts compatible with §6

§6 says the chrome is monochrome and colour means state. A chart needs eight hues for
identity. Those two statements are reconciled by position, not by intensity:

> **Hue carries identity only inside a chart's own frame. Everywhere else, colour means
> state.**

A chart is a bounded region with a title, a legend and a table view. A state chip is not.
A reader never has to decide which meaning a colour is carrying, because the frame says.

The first attempt at this rule was to desaturate the chart palette so a state colour always
outranked a data series. It does not work: below a chroma of roughly 0.10 a hue reads as grey
and stops distinguishing anything, so a desaturated categorical palette fails at its only job.

**A status colour is never a series colour, and a series colour is never presented as a
state.** Where a series sits in the same hue family as a state — slot 4 is yellow, slot 6
green, slot 8 red — the separation is carried by §7's existing rule that a state always ships
with a word or an icon, never hue alone.

### 13.2 Categorical — eight slots, fixed order

The order is the colourblind-safety mechanism. It is not cosmetic and it is never reordered
or cycled. A ninth category folds into `series-other`.

| Slot | Hue | Light | on `#FCFCFA` | Dark | on `#121210` |
| --- | --- | --- | --- | --- | --- |
| 1 | blue | `#0E76E4` | 4.32:1 | `#1480F8` | 4.91:1 |
| 2 | orange | `#F65D13` | 3.15:1 | `#F05907` | 5.52:1 |
| 3 | aqua | `#2AA374` | 3.09:1 | `#05A672` | 6.03:1 |
| 4 | yellow | `#BE831E` | 3.15:1 | `#BE8109` | 5.68:1 |
| 5 | magenta | `#EB5D98` | 3.12:1 | `#EB4F94` | 5.45:1 |
| 6 | green | `#1F811C` | 4.84:1 | `#179809` | 4.94:1 |
| 7 | violet | `#4C27C0` | 8.64:1 | `#6855DE` | 3.51:1 |
| 8 | red | `#F90F2D` | 3.96:1 | `#EB4848` | 4.98:1 |
| — | Other | `#a5a4a2` | — | `#646360` | — |

**These are re-stepped for this product's surfaces, not adopted from a reference palette.**
The reference instance leaves three light slots below 3:1 and covers them with the "relief
rule" — legal only where visible labels or a table view exist. That was declined here: a
palette that depends on an escape hatch to be legible is a palette that fails the moment
someone removes the table view.

| | Light `#FCFCFA` | Dark `#121210` |
| --- | --- | --- |
| Worst adjacent, protanopia/deuteranopia simulated | ΔE 9.4 | ΔE 9.6 |
| Worst adjacent, tritanopia | ΔE 9.7 | ΔE 10.1 |
| Worst adjacent, normal vision | ΔE 17.8 | ΔE 18.9 |
| Contrast against surface | **all eight ≥ 3:1** | **all eight ≥ 3:1** |

Re-stepping improved colourblind separation rather than costing it — 9.1 → 9.4 light and
8.4 → 9.6 dark, with tritanopia improving most (5.8 → 9.7 light). The only measure that fell
is the normal-vision worst pair, 19.6 → 17.8, comfortably above its floor of 15.

The dark steps preserve the light palette's **lightness structure** rather than sitting flat
at the band ceiling. Lightness variation does real separation work; a palette where every slot
shares one lightness relies on hue alone, which is exactly what a colourblind reader lacks.

Two series sit near a reserved state colour: series green against `ok` (ΔE 10.2) and series
red against `danger` (ΔE 14.4). Both are far better separated than the reference instance
manages — it documents red against critical at ΔE 4.8 — and §7's icon-and-label rule covers
the remainder.

### 13.3 Diverging — blue ↔ orange, never blue ↔ red

| | Light | Dark |
| --- | --- | --- |
| Positive, outermost → inner | `#184f95` `#2a78d6` `#5598e7` `#86b6ef` | `#358EFD` `#1177E7` `#0763C4` `#0050A2` |
| Neutral midpoint | `#ECEBE8` | `#2E2E2B` |
| Negative, inner → outermost | `#FF9067` `#F16223` `#C94905` `#883004` | `#8C3001` `#AB3D04` `#CB4B08` `#EA5B18` |

Red is the obvious negative pole and is forbidden here by §6: an expense line is negative every
month, a loss month is not a failure, and red is what marks a rejected batch. A diverging
column chart of monthly net result would otherwise paint half the year in the alarm colour.

Poles separate at ΔE 21.0 light and 30.4 dark under simulated protanopia. Both arms are
monotone in lightness with visible steps and clear the near-surface floor in both modes.

### 13.4 Sequential — one hue, and rarely the right answer

Blue, in ordered lightness steps, for continuous magnitude. **No Demo chart uses it.**

The sorted expense bar looks like a case for it and is not: bar length already encodes
magnitude, so colouring the bars by their value spends the identity channel re-encoding what
the reader can already see. Those bars take slot 1 at one step, and the legend is the title.

### 13.5 The rules that apply to every chart

- **No chart carries two vertical scales.** Two measures of different magnitude become two
  charts, small multiples, or a common index.
- **A legend wherever two or more series appear**, and four or fewer are also directly
  labelled. Identity is never colour alone.
- **Every chart has a table view.** `WORKFLOW.md` §5.3. It is a way to read the figures, not
  a crutch for the palette: every slot clears 3:1 unaided (§13.2).
- **Colour follows the entity, never its rank.** A filter that removes series must not
  repaint the survivors, or the reader is comparing something other than what they think.
- **Text wears text colours**, never a series colour. A coloured mark beside a label carries
  the identity.
- **Dark is a chosen set of steps**, not a filter or an inversion. `WORKFLOW.md` §5.3.
- **Never animate a figure**, in a chart or out of one — §5. A chart may draw itself in.

---

## 14. What this must not look like

*Added 2026-09-08, after the first components were built and read as generated
rather than designed.*

§2 says minimal means less decoration. That is a principle, and principles lose
to defaults — every component library, every starter template and every
generated draft converges on the same handful of shapes, and reaching for one is
easier than deciding. This section names them, so that using one is a choice
somebody made rather than a default nobody noticed.

The reference for this product is **financial print** — annual reports, the
tables in a broadsheet, a type specimen. It is not a SaaS dashboard. An
accountant reading twelve periods across is doing what a reader of a printed
table does, and that tradition solved these problems already.

| Reject | Because | Instead |
| --- | --- | --- |
| A tinted pill behind a status word | The single most template-shaped element there is. It spends a surface on two words and makes nine states look like nine buttons | The word in the state's colour, with its mark. Three channels in the height of one line |
| A card around every group | Chrome bought with padding, which §1.1 says costs rows per screen | A hairline rule, and the space the rule already implies |
| Evenly gapped grids of equal rounded swatches | Nothing is more important than anything else, so the reader has to do the ranking | A butted band, hard-edged, so the steps read against each other |
| A heading with a muted subtitle beneath it, at every level | Uniform rhythm reads as filler. Real hierarchy has few large things and many small ones | A narrow label column with the material hanging off it. Asymmetry is the layout |
| Radius on everything | A radius applied uniformly is not a decision | 3px on controls, 6px on panels, nothing on a swatch or a rule |
| A drop shadow to separate anything | §5 already restricts shadows to true overlays | A surface step, or a hairline |
| Centred single columns of even blocks | The shape of a page with no opinion | A measure, a label column, and alignment that means something |
| Icon beside every label | §2 already bans it where the label is clear | An icon only where it carries what the word cannot — a state, a direction |

Two positive rules, which cost nothing and are the fastest way to stop looking
generic:

- **Set the numbers properly.** Tabular figures in every column of figures, the
  minus sign not a hyphen, the currency in the header rather than the cell.
  §4 requires all three, and they are the details a reader of financial tables
  notices immediately and cannot name.
- **Let type carry hierarchy.** Weight, size and case, in that order. A label in
  small letterspaced capitals above a figure does the work a card was reached
  for, in one line and no chrome.
