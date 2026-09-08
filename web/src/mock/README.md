# Mock screens

Clickable mockups of the Demo screens. **Not production code.** Nothing here is
imported by `src/main.tsx`, `src/router.tsx` or `src/index.css`.

## Run

```sh
cd web && npm run dev
```

Then open <http://localhost:5173/mock.html>. The real app stays at `/`.

## What works

| Screen | Interaction |
| --- | --- |
| Reports | Click any figure to open the drill-down panel. Only Logistics · March carries a transaction list — see below |
| Imports | Click a batch row. The rejected one opens its validation report and error list |
| Review | Keyboard-first: `1`–`6` pick a category, `↵` approves the whole counterparty group, `T` internal transfer, `N` not in the P&L, `↑` `↓` move, `Esc` clears. Approving empties the queue and the rail badge follows |
| Token sheet | The palette, the type ramp and the state vocabulary |
| Top bar | `EN` / `RU` re-formats every figure (space group, comma decimal) and re-words every state chip |

## What is deliberately fake

- **Amounts** are invented, but they are consistent: the totals, subtotals,
  percentages and the reconciliation strip all compute from `data.ts` rather
  than being typed in. Change a value and the report still adds up.
- **The drill-down transaction list** exists for one cell only. Populating every
  cell would mean inventing a few hundred transactions, and
  `IMPLEMENTATION_PLAN.md` §3 bans a demo on invented data.
- **The Russian strings** in `data.ts` are a draft and need a native check
  before change 5.1b freezes the message catalog.
- **Mock copy lives in `data.ts`, not in `src/i18n.ts`.** The screens carry
  roughly sixty throwaway strings, and putting them in the production catalog
  would leave them behind when this directory is deleted. The `Locale` type is
  imported from `src/i18n.ts`, so the repo still has one locale concept.

## What is real, and worth keeping

- `money.ts` formats from **int64 minor units as strings** and sums in `BigInt`.
  No float touches an amount. The production formatter (change 5.1a,
  `src/money.ts`) must work the same way — this file is the sketch of it.
- `mock.css` holds the token layer from `docs/DESIGN.md` §3 as plain custom
  properties. Change 5.1a lifts those values into `src/index.css` as a Tailwind
  v4 `@theme` block. Written this way on purpose, so the mock cannot drag the
  app's styling around.

## When to delete this

When changes 5.2a, 5.2b and 5.2c have landed the real screens. Delete the whole
directory plus `web/mock.html`. Nothing else references either.
