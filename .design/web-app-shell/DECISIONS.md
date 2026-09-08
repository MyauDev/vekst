# 5.1a `add-web-app-shell` — decisions

Date: 2026-09-07 · Phase 1 of the design flow, `grill-me`.
Companion documents: `docs/DESIGN.md` (the brief), `docs/FRONTEND_PLAN.md` §3.

> This file stands in for a design brief. `docs/DESIGN.md` is the brief and it
> is not restated here. What follows is only what that document left open, or
> got wrong, and what was decided about it on 2026-09-07.

---

## 1. Resolved from the codebase, not asked

| Question | Answer | Evidence |
| --- | --- | --- |
| `DESIGN.md` §12 Q4 — the literal-colour lint rule has no home | `scripts/check-web-tokens.sh`, called from `make lint` | `scripts/check-identity-queries.sh` is the same shape: a grep-based boundary check holding an invariant no type system can |
| `FRONTEND_PLAN.md` §7 Q1 — does the Demo need sign-in? | Moot. 1.2 shipped `SignIn.tsx` and `SignedIn.tsx` | The question existed to decide whether to cut 1.2 |
| Is the i18n mechanism 5.1a's or 5.1b's? | 5.1b. 5.1a adds its keys to the existing hand-rolled `t()` | `src/i18n.ts` says 5.1 owns it; `FRONTEND_PLAN.md` §3 is a day newer and splits it deliberately |
| Can the mock be ported to production? | No. It is a visual spec to read | `src/mock/ui.tsx` is inline `style={{}}` throughout, on purpose, so it could not drag the app's styling around |

The measurements in the mock are worth keeping even though the code is not:
208 px rail, 48 px top bar and rail header, 32 px nav rows, the pending badge
in the rail beside Review.

## 2. Decided

### 2.1 Inter, self-hosted

`DESIGN.md` §4 commits to Inter and gives the reason (Cyrillic).
`src/mock/mock.css` ships IBM Plex. The document is the constraint; the mock
drifted. Self-hosted rather than from a CDN: it matches the same-origin posture
already taken in `vite.config.ts` (design D6), needs no egress from the
cluster, and keeps the CSP simple. Two subsets, latin and cyrillic.

Closes `DESIGN.md` §12 Q3. **Follow-up:** delete the IBM Plex line from
`mock.css` so one answer exists in the repository.

### 2.2 5.1a migrates the existing components and turns the rule on

There are 22 literal colour classes across all five production components —
`App.tsx`, `SignIn.tsx`, `SignedIn.tsx`, `HealthCard.tsx`, `router.tsx`.
`DESIGN.md` §3 calls each one a defect. 5.1a ports them onto the semantic
tokens and then enables `check-web-tokens.sh` green.

The ordering matters: a lint rule that ships disabled is not a lint rule, and
`DESIGN.md` §3 prices dark mode at 0.5 days *only* while no component writes a
literal colour. Roughly +0.5 day on a 1.5-day change.

`App()` at `src/App.tsx:28` is deleted with them. It duplicates the shell
markup now in `router.tsx`'s `rootRoute`, is unreachable from `main.tsx`, and
survives only because `App.test.tsx` mounts it. Two shells, one reachable only
from tests, is the wrong thing to have while building the real one.

### 2.3 Organisation and entity come from an extended `User`

`proto/vekst/v1/identity.proto` already states that change 1.1 extends the
`User` message. Change 1.1's tasks have six sections and none of them touch
proto, and no change in `IMPLEMENTATION_PLAN.md` adds an organisation or entity
listing RPC. So `DESIGN.md` §8's top bar has no data source for two of its
three selectors.

Fixed where the document already said it belonged: **change 1.1 gains a task to
extend `User`** with the organisation id and name and the entity id. 5.1a then
reads both from `GetCurrentUser`. v1 creates one entity per organisation
(`CLAUDE.md`), so the entity selector renders a single fixed value — the slot
is present, which is what §8 asks for, without inventing a picker for a list of
one.

This keeps `/proto` work inside Track B, where `.github/CODEOWNERS` wants it.
A Track A web change cannot quietly absorb it: `/proto` needs both reviewers.

**Follow-up, outside 5.1a:** add that task to
`openspec/changes/add-tenancy-and-rls/tasks.md`.

### 2.4 Locale is a stored preference, not a search param

`localStorage`, falling back to `navigator.language` as `resolveLocale`
already does. `FRONTEND_PLAN.md` §5 puts organisation, entity and period in
search params so a drill-down link is shareable; language is not part of what
such a link identifies. A shared link says which numbers to look at. The
reader's language is about the reader.

### 2.5 The three routes ship real product empty states

Not "not built yet" placeholders. Each route renders what a genuine new
customer sees on day one, with no call to action where the action is not wired
— Imports cannot offer upload while 2.1 is blocked.

This retires `FRONTEND_PLAN.md` gap 9, which notes the nine empty, loading and
error states are unbudgeted and are "most of what a reviewer sees on a Demo
day, because the happy path is fast". Written this way they survive into 5.2a,
5.2b and 5.2c unchanged instead of being thrown away.

### 2.6 A dev-only `/tokens` route

`DESIGN.md` §7's three axes are nine state components, and 5.1a builds all of
them while no screen renders any of them until 5.2. `TokensScreen` from the
mock is ported into the real app, behind a dev-only flag and off the production
rail.

It gives the nine components a caller, makes §9's contrast table verifiable
against what actually shipped rather than against a document, and shows drift
between `DESIGN.md` §3 and `src/index.css` visually. Leaving it in the mock
does not do this: the mock renders `mock.css`, so it can agree with itself
while disagreeing with the app.

## 3. Still open, for later phases

| # | Item | Phase |
| --- | --- | --- |
| 1 | The type ramp in `DESIGN.md` §3 has sizes and no line heights. Tailwind v4 pairs them as `--text-*--line-height` | 4 |
| 2 | §5 says "nothing above 32 inside the app", but Tailwind v4's dynamic spacing means `p-96` still compiles. Either the lint script catches it or the rule is unenforced | 4 |
| 3 | `mock.css` carries `--vk-hairline` and `--vk-accent-surface`, which `DESIGN.md` §3 does not. Promote or drop | 4 |
| 4 | Period search-param shape and the default range when absent | 3 |

## 4. Untouched by this change

`DESIGN.md` §12 Q1 and `FRONTEND_PLAN.md` §7 Q4 — the Russian state words need
a native check before 5.1b freezes the catalog. 5.1a writes the nine state
words, so the draft `ru` column is used here first; it is still a draft.

`DESIGN.md` §12 Q2 and `FRONTEND_PLAN.md` §7 Q3 — the landing page has no owner.
Out of scope, still unowned.

`FRONTEND_PLAN.md` §7 Q5 — whether a viewer sees the review queue read-only.
That is 5.2b.

---

# Scope change, 2026-09-07 (same day)

The work is no longer change 5.1a alone. It is **both spaces** — the landing
page and the whole authenticated application — built as production code against
mock data, with the backend connected later.

`openspec/config.yaml:7` already calls these "two blocks: a public marketing
site and an authenticated dashboard", and `DESIGN.md` §1 already calls them
Register A and Register B sharing one token layer. The framing is not new. The
schedule is: `WORKFLOW.md` §0 puts the landing at Product and dark mode at
Product, and both are being pulled forward deliberately.

## 5. What Unlumen actually is

`DESIGN.md` §1 names `https://ui.unlumen.com/` as "the design reference and the
source of component primitives". Read on 2026-09-07, its catalogue is an
**animation library**: Aurora Card, Blob Card, Gooey SVG Filter, Magnetic
Button, Glow Button, Scramble Text, Tilt Card, Orbiting Skills, WebGL shader
backgrounds. There is no data table, no form system and no dense layout
primitive in it.

§1's hedge — "its defaults are Register A" — is correct but understates the
consequence:

- **Register A (landing).** Unlumen carries it. This is what the library is for.
- **Register B (app).** Four useful pieces: `Kbd` for the review queue's
  always-visible keyboard legend (§8), `Theme Switch` for the white/black
  toggle, `Shimmer Skeleton` for the loading states of `FRONTEND_PLAN.md` gap 9,
  and `Command Menu`. The P&L table, the review queue and the imports list are
  built here, not adopted.

This agrees with what §11 already rejected — "adopting a component library's
spacing and type scale as-is" — and now has evidence behind it.

## 6. Mock data: a typed, proto-shaped data layer

One module per screen under `web/src/data/`. Hand-written TypeScript interfaces
shaped like the proto messages that will eventually exist — money as `int64`
minor-unit **strings**, states as codes, never a JS `number` for an amount —
plus fixtures behind them. Components import the module and never touch a
transport.

Connecting the backend replaces the module body. It does not touch a component.

Rejected: **proto-first**. Defining `ImportService`, `ReviewService` and
`ReportService` now would give a stronger guarantee — CI's codegen drift check
would police the contract, and there would be no port at all. It is rejected
because it means designing the APIs of changes 2.1, 2.3, 3.3, 4.1 and 4.2
before those changes are specified, and `/proto` needs both reviewers per
`.github/CODEOWNERS`. That is Track B's design work, not this one's.

Rejected: **fixtures inline in components**. This is what `src/mock/` already
did, and its own README says it is not production code.

The existing `src/testTransport.ts` stays as it is. It is the right mechanism
for Health and Identity, which do have protos, and `FRONTEND_PLAN.md` §5 chose
it deliberately over MSW.

## 7. Monochrome chrome, colour reserved for state

Every widget — navigation, tables, buttons, cards, inputs, chips — is black,
white or grey. **The accent stops being blue and becomes ink.** `DESIGN.md` §6
is amended: the accent's job is still "the primary action, one per view", but
it is carried by weight and surface rather than hue.

**The three state colours survive.** `DESIGN.md` §2 lists the reason a figure is
blocked, and the import batch counts and states, among the things that may never
be removed. In a fully monochrome interface a rejected import and an imported
one differ by a single word. That is the one place in this product where a
reader must not have to read carefully.

§7's rule is unchanged and now carries more weight: state is never communicated
by colour alone. The word and the icon are what survive print, and §9 requires
the P&L to print in black and white.

Rejected: **fully monochrome, including state.** Defensible — §7 guarantees it
is safe, because colour was never the only channel. Rejected because it spends
the product's one legible alarm on an aesthetic preference.

Rejected: **monochrome plus a single action accent, no state colour.** It
inverts §6's priority, spending the colour budget on buttons instead of meaning.

## 8. Dark mode is in

"White or black" is light and dark. `WORKFLOW.md` §0 has dark mode at Product
and `DESIGN.md` §11 rejected it for the Demo; both are overridden here.

This is affordable for exactly the reason §3 gave: the price is 0.5 days **only
while every component uses semantic token names**. It makes decision 2.2 — the
22-violation migration and `check-web-tokens.sh` — a prerequisite rather than a
nicety. The token layer now needs two complete value sets, and §3's oklch values
are light-only.

## 9. One application, two spaces

`/` is the public landing. `/app` is the authenticated dashboard. One Vite
bundle, one `@theme` block, one set of tokens.

`ARCHITECTURE.md` §8 records `/site — Astro static marketing site (from
Commercial)`, and `config.yaml:67` says "Astro static, separate bundle". §9
Rejected alternatives does **not** defend it: unlike the classifier/database
boundary, it is a layout note with no recorded reasoning, which is why
revisiting it is cheap.

Chosen because `DESIGN.md` §1's "both registers use the same tokens" becomes
literally true inside one build, rather than a convention maintained by hand
across two. Unlumen is React + Motion and drops in natively.

The static extraction stays scheduled at Commercial and nothing here blocks it.
What would block it is a landing page that reaches into application state; it
must not.

## 10. The constraint documents get amended, not left behind

`CLAUDE.md` treats `docs/DESIGN.md` as a constraint on future work. A constraint
document that quietly disagrees with the running application is worse than none,
because the next person trusts it.

| Document | Amendment | When |
| --- | --- | --- |
| `DESIGN.md` §6 | The accent is ink, not blue | With the token layer |
| `DESIGN.md` §7 | Unchanged, but state colour is now the only colour | With the token layer |
| `DESIGN.md` §9 | A second contrast table for dark | With the token layer |
| `DESIGN.md` §11 | Dark mode is no longer rejected; record why it moved | With the token layer |
| `DESIGN.md` §12 | Q2 answered — the landing is a route in `web/`, for now | With the landing |
| `WORKFLOW.md` §0 | Landing and dark mode pulled forward from Product | With the landing |
| `ARCHITECTURE.md` §8 | `/site` is still the Commercial target; `/` is the interim home | With the landing |

## 11. What the earlier decisions do under the new scope

| # | Decision | Status |
| --- | --- | --- |
| 2.1 | Inter, self-hosted | Holds |
| 2.2 | Migrate the 22 literal colours, enable the rule | Holds, and is now a **prerequisite** for dark mode rather than hygiene |
| 2.3 | Organisation and entity from an extended `User` | Holds |
| 2.4 | Locale is a stored preference | Holds. The theme choice is stored the same way |
| 2.5 | Real product empty states | Holds, across more screens |
| 2.6 | Dev-only `/tokens` route | Holds, and is worth more: it is where two complete palettes get compared |

---

# Phase 4 — the token layer, 2026-09-08

`web/src/index.css` is now the token layer. `docs/DESIGN.md` §3 is amended to
say so and to point here for the reasoning.

## 12. The three items §3 left open are closed

**Line heights.** The ramp had sizes and no leading, which left every component
to invent its own. Each size now carries `--text-*--line-height`, and the four
display sizes carry `--text-*--letter-spacing` as well: 1.45 at 11px down to
1.05 at 48px, tightening as the type grows, which is how a ramp is supposed to
behave.

**"Nothing above 32" is now enforceable.** It could not live in the token layer
— Tailwind v4 generates spacing utilities dynamically, so `p-96` compiles
whatever `@theme` says. It lives in `scripts/check-web-tokens.sh`, which reads
the numeric step out of every padding, margin, gap and space utility and fails
above step 8. Register A is exempt from that rule and only that rule: §1 gives
the landing sections at 64–96px.

**Both tokens the mock invented are dropped.**

| Token | Decision | Why |
| --- | --- | --- |
| `--vk-hairline` (94%) | Dropped | It is a third border weight, sitting between `surface` and `border`. §5 says "one border level per view"; two weights is already the generous reading |
| `--vk-accent-surface` | Dropped | It was a tinted blue wash, and the accent is no longer a hue. The mock's own `Rail` used `sunken` for the active navigation item rather than this token, so `surface-sunken` already does the job |

## 13. What the token file does that the document did not specify

**Register A type sizes.** §3's ramp caps at 24px and calls it "the product
maximum", but §1 gives the landing "up to 48px". The ramp therefore had no
sizes for half the product. Added: 30, 36 and 48px, marked as Register A and
not to appear behind sign-in.

**`@theme inline`, not `@theme`.** The semantic names map onto raw variables
through `@theme inline` so a utility emits `var(--vk-surface)` and resolves at
the element. A plain `@theme` resolves once against `:root`, and the dark
palette would have silently done nothing. Verified against the compiled output
rather than assumed.

**A print block.** §9 requires the P&L to print readably in black and white.
Without an override a dark-mode print is a page of toner, so `@media print`
forces the light ground regardless of theme.

**`prefers-reduced-motion`.** Not in the document. §5 already restricts motion
to the drill-down panel, but Register A is allowed motion and Unlumen's pieces
are motion, so the escape hatch has to exist before those land.

**A third theme state.** `[data-theme]` absent means follow the system;
`light` and `dark` are explicit choices. §11 assumed one palette and so never
described the toggle's third position.

## 14. Contrast was measured, not asserted

Every pair in §9's amended table was computed from the oklch values by
converting through OKLab to linear sRGB to relative luminance. Both palettes
clear AA at 4.5:1 on every text pair.

The light state colours were deepened — `ok` 52%→44%, `warn` 54%→47%,
`danger` 52%→50%. The published originals passed at 4.69, 4.59 and 5.27, which
is above the line with nothing to spare, and in a monochrome interface these
three are the only colour carrying meaning. They now sit at 6.61, 6.17 and
5.74.

The dark ground is `oklch(18%)` = `#121210`, not `#000`. White text on true
black blooms, and a P&L is read in figures.

## 15. The rule is on, and it is currently red

`scripts/check-web-tokens.sh` is wired into `make lint`. It fails today, naming
18 lines across all five production components — the 22 literal colours from
decision 2.2.

Wired now rather than after the migration, deliberately. A failing check that
names every line to fix is a task list that cannot be forgotten;
`STATUS-2026-09-06.md` §4 is a list of small correct things that were left for
later. The migration is the first build task, and `make lint` goes green with it.

---

# Charts, 2026-09-08

## 16. Charts came into scope, and forced a deferred decision

Charts were brought forward from Commercial by decision. I recommended against it and was
overruled; the reasoning below is what the decision actually costs and buys, recorded so the
next reader does not have to reconstruct it.

**What it forced.** `DESIGN.md` §0 listed "chart colour ramps" among the things it explicitly
did not decide. A monochrome interface whose only colour is state cannot absorb eight
categorical hues without a rule for how the two relate, so the deferral could not survive
charts arriving. The rule is now §13.

**The resolution is containment, not desaturation.** The instinct was to give charts
low-chroma colour so a state chip always outranks a data series. That is measurably wrong:
below a chroma of about 0.10 a hue reads as grey and stops distinguishing anything, so a
desaturated categorical palette fails at its only job. Hue carries identity **inside a
chart's frame**; outside one, colour means state. A chart is bounded, titled, legended and
has a table view. A state chip is none of those.

**The palette was validated, not chosen.** Every slot was checked against a lightness band, a
chroma floor, adjacent-pair separation under simulated protanopia and deuteranopia, and
contrast against Vekst's own two surfaces rather than a reference pair. Worst adjacent CVD
separation is ΔE 9.1 light and 8.4 dark, against a target of 8.

**One reordering was tried and rejected on evidence.** Slots 4, 6 and 8 are yellow, green and
red — the three state families. Leading instead with blue, orange, aqua, magenta and violet
would keep a five-series chart clear of them. Two candidate orders were measured: the first
collapsed in dark mode (magenta against aqua, ΔE 1.6 under deuteranopia — indistinguishable),
and the second passed but pushed both modes into the 6–8 warn band. Real colourblind
separation is worth more than the cosmetic worry, and §7's icon-and-label rule already
addresses the worry directly.

**Diverging could not use the standard pair.** Blue ↔ red is the conventional diverging
choice and is forbidden here by §6: a diverging column chart of monthly net result would paint
every loss month in the colour that marks a rejected import. The negative arm is orange,
derived at the categorical orange's hue and lightness-matched to the blue arm.

**What this does to the schedule.** `FRONTEND_PLAN.md` §3 budgets `add-charts-echarts` at one
day as a Product item. That was for charts against a decided palette. The palette is now
decided, so the estimate is closer to honest than it was — but ECharts is roughly 1 MB, six
chart forms are specified in `WORKFLOW.md` §5.3, and each needs a table view. Section 8 of the
task list is eighteen tasks, and it is the largest single section in the change.

## 17. What phase 4 produced, in one place

| Artefact | Where |
| --- | --- |
| Token layer, two palettes, charts included | `web/src/index.css` |
| Enforcement | `scripts/check-web-tokens.sh`, called from `make lint` |
| Colour contrast, both palettes | `docs/DESIGN.md` §9 |
| Chart palette and its rules | `docs/DESIGN.md` §13 |
| Why each value | `openspec/changes/add-web-experience/design.md` D2–D4, D12 |

## 18. The chart palette was re-stepped for readability, 2026-09-08

The first pass adopted the reference palette's values wholesale. It left three light slots
below 3:1 against our surface — aqua 2.74, magenta 2.62, yellow 2.11 — legal under the
reference's "relief rule", which permits sub-3:1 fills wherever visible labels or a table view
exist.

That was declined on the readability brief. A palette that needs an escape hatch to be legible
fails the day someone removes the table view, and the dependency is invisible until then.

Every hue was held fixed and its lightness walked down until the mark cleared 3:1, taking the
most chroma that stayed in gamut at that lightness. The dark set then mapped the light
palette's lightness *structure* into the dark band, rather than parking every slot at the band
ceiling — a first attempt did exactly that and separated worse, because a palette with one
lightness leans entirely on hue, which is what a colourblind reader lacks.

| | Light | Dark |
| --- | --- | --- |
| Protanopia/deuteranopia, worst adjacent | 9.1 → **9.4** | 8.4 → **9.6** |
| Tritanopia, worst adjacent | 5.8 → **9.7** | 8.7 → **10.1** |
| Normal vision, worst adjacent | 19.6 → 17.8 | 19.3 → 18.9 |
| Slots below 3:1 | 3 → **0** | 0 → **0** |

Readability and colourblind safety moved together rather than trading off, which is the
uncommon case and worth recording: the assumption that accessibility work costs vividness did
not hold here. Only the normal-vision worst pair fell, and it stays clear of its floor of 15.
