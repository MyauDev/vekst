# Design — `add-web-experience`

Date: 2026-09-08. Companion: `.design/web-app-shell/DECISIONS.md` (the long form),
`.design/web-app-shell/INFORMATION_ARCHITECTURE.md` (routes, flows, glossary),
`docs/DESIGN.md` (amended by this change), `docs/FRONTEND_PLAN.md`.

---

## D0. This change carries several capability deltas

`CLAUDE.md` says one change is one capability delta. This one covers 5.1a, 5.1b, 5.2a, 5.2b
and 5.2c plus an unowned landing page. It was proposed this way by decision on 2026-09-08.

The seams, if it has to be split: §2 (shell), §5 (landing) and §6–§7 (Imports, Review). Each
is a section of `tasks.md` with no forward dependency on the sections after it.

## D1. Mock data lives behind a typed module, not behind a fake transport

`web/src/data/` holds one module per screen. The interfaces are shaped like the proto messages
that will eventually carry them — money as `int64` minor-unit strings, states as codes — and
fixtures sit behind them. Components import the module.

**Rejected: proto-first.** Defining `ImportService`, `ReviewService` and `ReportService` now
and backing them with `createRouterTransport` would be stronger — CI's codegen drift check
would police the contract and there would be no port at all. It is rejected because it means
designing the APIs of changes 2.1, 2.3, 3.3, 4.1 and 4.2 before those changes are specified.
That is Track B's design work, and `/proto` needs both reviewers.

**Rejected: fixtures inside components.** This is what `web/src/mock/` already did, and its
own README says it is not production code.

`web/src/testTransport.ts` is untouched. It is the right mechanism for Health and Identity,
which do have protos, and `FRONTEND_PLAN.md` §5 chose it over MSW deliberately.

## D2. The chrome is monochrome; state keeps its colour

Every widget is black, white or grey, and the accent is ink rather than blue.

`DESIGN.md` §6 previously reserved colour for three jobs and made the accent blue. Removing
the hue from the chrome satisfies the reason that rule existed — an accent matching a state
colour makes "this is a button" and "this passed" look alike — more completely than choosing
a different hue does.

**The three state colours survive.** `DESIGN.md` §2 lists the reason a figure is blocked, and
the counts and states of an import, among the things that may never be removed. In a wholly
monochrome interface a rejected import and an imported one differ by one word, and that is the
one place in this product where a reader must not have to read carefully.

**Rejected: fully monochrome including state.** §7 guarantees it is safe, since colour was
never the only channel. Rejected because it spends the product's one legible alarm on a
preference.

The light state values were deepened — `ok` 52%→44%, `warn` 54%→47%, `danger` 52%→50%. The
published originals passed at 4.69, 4.59 and 5.27, above the line with nothing to spare, and
these three are now the only colour carrying meaning.

## D3. Dark mode ships with the shell

`WORKFLOW.md` §0 had it at Product and `DESIGN.md` §11 rejected it for the Demo.

§3 always priced it at half a day *on the condition* that every component uses a semantic
token name. The condition was never enforced and had 22 violations. This change enforces it
(D6), which makes the price real rather than hoped for. Shipping the second palette while the
first is being written costs a set of values; retro-fitting it costs a rewrite.

The dark ground is `oklch(18%)` — a warm near-black, not `#000`. White text on true black
blooms, and this product is read in figures.

## D4. `@theme inline`, not `@theme`

The semantic names map onto raw variables through `@theme inline`, so a utility emits
`var(--vk-surface)` and resolves at the element.

A plain `@theme` resolves once against `:root`. The dark palette would then have done nothing
at all while looking correct in the source — the failure mode is silent, which is why this is
recorded rather than left as a detail. Verified against compiled output.

## D5. One bundle, two spaces

`/` is the landing; `/app/*` is the dashboard; one Vite build, one `@theme` block.

`ARCHITECTURE.md` §8 records `/site — Astro static marketing site (from Commercial)` and
`config.yaml` says "separate bundle". §9 Rejected alternatives does not defend it: unlike the
classifier/database boundary it is a layout note with no recorded reasoning.

Chosen because `DESIGN.md` §1's "both registers use the same tokens" becomes a fact inside one
build rather than a convention maintained by hand across two. Unlumen is React and Motion and
drops in natively.

The Commercial extraction stays scheduled and nothing here blocks it. What *would* block it is
a landing page that reads application state, so the landing must not.

## D6. The rule goes on before the code is clean

`scripts/check-web-tokens.sh` was wired into `make lint` while it still failed on 18 lines.

A failing check that names every line to fix is a task list that cannot be forgotten.
`STATUS-2026-09-06.md` §4 is a list of small correct things that were left for later.
The migration is §1, the first build task, and `make lint` goes green with it.

## D7. Tenant context comes from the session; the figure comes from the URL

`FRONTEND_PLAN.md` §5 put organisation, entity and period in search params together, for
shareable drill-down links. This change puts **period** there and reads **organisation and
entity from the session**.

In v1 a user belongs to one organisation, so the value is not a choice, and a URL carrying a
tenant identifier the session already fixes invites the next reader to trust it. RLS would
then faithfully isolate the transaction to whichever organisation the URL named. It promotes
to a search param the day membership becomes plural, which is a smaller change than removing
a parameter that has been trusted.

A link says which figures to read. It does not say whose.

## D8. The drill-down is a route

`/app/reports/pnl/cell/$category/$period` renders the panel over the report, which stays
mounted.

`DESIGN.md` §8 asks for a panel; `FRONTEND_PLAN.md` §5 asks for shareable links and a working
back button. A nested route is the one shape that gives both, and it makes the back button
close the panel rather than leave the report.

## D9. Unlumen is the landing's library, not the application's

`DESIGN.md` §1 names it "the design reference and the source of component primitives" and
hedges that "its defaults are Register A". Read on 2026-09-07, its catalogue is an animation
library — Aurora Card, Gooey SVG Filter, Magnetic Button, Scramble Text, WebGL backgrounds.
There is no data table, no form system and no dense layout primitive in it.

It carries the landing. It contributes four pieces to the application: `Kbd` for the review
legend, `Theme Switch`, `Shimmer Skeleton`, `Command Menu`. The P&L table, the review queue
and the imports list are built here.

## D10. The `User` message is extended in this change, not in 1.1

`proto/vekst/v1/identity.proto` states that change 1.1 extends `User`. Change 1.1's tasks have
six sections and none touches proto, and no change in `IMPLEMENTATION_PLAN.md` adds an
organisation or entity listing RPC — so `DESIGN.md` §8's top bar has no data source for two of
its three selectors, in any planned change.

The extension is carried here because this is the change that needs it, and it is small: the
organisation identifier and name, and the entity identifier, on a message that already exists.
It requires both reviewers, and it must land after `add-tenancy-and-rls` creates the tables it
reads from.

**Rejected: adding a listing RPC.** v1 gives one organisation per user and one entity per
organisation. A list endpoint for a list of one is a surface to secure for no benefit.

## D11. `DataTable` and `PnlTable` are separate components

Freezing a column, freezing a header and scrolling horizontally is the P&L's whole job, and
nothing else in the product does it. Folding it into a general table makes the most complex
component in the codebase the one used correctly in exactly one place.

## D12. The chart palette is validated, not chosen

Charts came into scope on 2026-09-08, having been a Commercial item. That forces the decision
`DESIGN.md` §0 deferred — it "does not decide chart colour ramps" — and it is the decision
most in tension with D2, because a monochrome interface whose only colour is state now has to
absorb eight categorical hues.

**The resolution is containment, not desaturation.** My first instinct was to give charts
low-chroma colour so a state chip always outranks a data series. That is wrong and the
validator says so: below a chroma of about 0.10 a hue stops doing identity work and reads as
grey, so a desaturated categorical palette fails at the one job it has. The rule is instead
positional: **hue carries identity only inside a chart's frame; everywhere else colour means
state.** A chart is a bounded region with a legend and a table view. A state chip is not.

**The palette holds the reference hues and slot order but re-steps every value for Vekst's
own surfaces.** The reference instance leaves three light slots below 3:1 and covers them with
the relief rule — legal only where visible labels or a table view exist. Declined: a palette
that needs an escape hatch to be legible fails the moment someone removes the table view, and
readability was the stated goal.

| | Light `#FCFCFA` | Dark `#121210` |
| --- | --- | --- |
| Worst adjacent, protanopia/deuteranopia | ΔE 9.4 *(was 9.1)* | ΔE 9.6 *(was 8.4)* |
| Worst adjacent, tritanopia | ΔE 9.7 *(was 5.8)* | ΔE 10.1 *(was 8.7)* |
| Worst adjacent, normal vision | ΔE 17.8 *(was 19.6)* | ΔE 18.9 *(was 19.3)* |
| Contrast vs surface | all 8 ≥ 3:1 *(was 3 below)* | all 8 ≥ 3:1 |

Re-stepping improved colourblind separation rather than trading against it; tritanopia gained
most. Only the normal-vision worst pair fell, and it stays well above its floor of 15.

The derivation held each hue fixed and walked lightness down until the mark cleared 3:1,
taking the most chroma that stayed in gamut at that lightness. The dark set then mapped the
light palette's *lightness structure* into the dark band rather than parking every slot at the
ceiling — a flat palette leans on hue alone, which is what a colourblind reader lacks.

**Rejected: reordering the slots to push the status hues to the tail.** Slots 4, 6 and 8 are
yellow, green and red — the three state families. An order leading with blue, orange, aqua,
magenta and violet would keep a five-series chart clear of them entirely. Two candidate orders
were validated: one collapsed in dark mode (magenta against aqua, ΔE 1.6 under deuteranopia —
indistinguishable), and the one that passed pushed colourblind separation into the 6–8 warn
band in both modes. Trading real colourblind separation for a cosmetic worry about a yellow
bar resembling a warning chip is a bad exchange, and the reference palette answers the worry
directly: a series colour beside a same-family status cue leans on the icon-and-label pairing,
never on hue. `DESIGN.md` §7 has required that pairing since it was written.

**Diverging is blue ↔ orange, not blue ↔ red.** The reference pair is blue ↔ red, and it
cannot be used here: `DESIGN.md` §6 forbids red for a negative figure, and a diverging column
chart of monthly net result would paint every loss month red — training the reader to ignore
the colour that marks a rejected import. The orange arm was derived at the categorical
orange's hue, lightness-matched to the documented blue arm, and both arms validate for
monotone lightness, step separation and contrast in both modes. The poles separate at ΔE 21.0
light and 30.4 dark under protanopia, well clear of the target.

**Sequential is defined and unused.** No Demo chart encodes continuous magnitude. The sorted
expense bar looks like a case for it and is not: bar length already encodes magnitude, so
colouring the bars by their value spends the identity channel re-encoding what length shows.
Those bars take one hue at one step.

## D13. Sign-in lands on `/app`, failure lands on `/signin`

Splitting the browser into a public surface and an authenticated one moves where
a completed sign-in must return, and the move is not optional.

`core` redirected to `/` for both outcomes. That worked while `/` rendered the
session check. Once `/` became a landing page that reads no session — which D5
requires, so the page stays separable into a static bundle — a completed sign-in
returned a signed-in person to a page inviting them to sign in. Nothing errored.
The session cookie was set correctly and the user simply could not tell.

A failed sign-in has the opposite problem: `fail()` carries a machine-readable
code in `?auth_error=`, and the landing page does not render it. The code sat in
the URL, invisible.

So there are two constants rather than one: `postSignInPath = "/app"` and
`signInPath = "/signin"`. Both remain fixed paths, never a caller-supplied
return URL — that parameter is how a redirect flow becomes an open redirect.

**Rejected: having the landing page check the session and redirect.** It needs
no change to `core` and it is the obvious fix. It is rejected because it makes
the public surface read application state, which contradicts this change's own
spec requirement and is exactly what would make the `/site` extraction a rewrite
rather than a move.

The coupling is now asserted from the web tests, which read the two constants
out of `signin.go` rather than restating them — the same move as the Ingress
prefix test and the currency exponent test. A constant copied into a test drifts
from the constant it copies.
