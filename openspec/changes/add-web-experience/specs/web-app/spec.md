## ADDED Requirements

### Requirement: One token layer holds every colour, and a check enforces it

The system SHALL define every colour, type size, spacing step, radius and motion value in a
single token layer at `web/src/index.css`, using semantic names that describe a role rather
than a value. No component SHALL write a literal colour class, a raw hexadecimal, `rgb()`,
`hsl()` or `oklch()` value, or the words `black` or `white` as a colour.

The build SHALL fail on a violation. A rule that is stated in a document and enforced by
nothing is not a constraint: `docs/DESIGN.md` §3 called a literal colour class a defect on
2026-09-06 and there were 22 of them across all five components three days later. The
enforcement SHALL live beside the other boundary checks that `make lint` runs.

#### Scenario: A literal colour class fails the build

- **WHEN** a component is committed containing `text-slate-900`
- **THEN** the token check exits non-zero
- **AND** it names the file and the line

#### Scenario: A raw colour value fails the build

- **WHEN** a component is committed containing a hexadecimal colour or an `oklch()` call
- **THEN** the token check exits non-zero
- **AND** the token layer itself is exempt, because holding those values is its purpose

#### Scenario: The application does not spend more than 32px on a gap

- **WHEN** a component under the application renders a padding, margin or gap above 32px
- **THEN** the token check exits non-zero
- **AND** the landing page is exempt from this rule and from no other

### Requirement: Two registers share one token layer

The system SHALL present a public register and an application register that differ in which
type sizes, spacing steps and element counts they use, and SHALL NOT give them separate token
layers. The application register SHALL cap type at 24px; the public register MAY use sizes up
to 48px, and those sizes SHALL NOT appear behind sign-in.

Minimal SHALL mean less decoration and never less information. White space in the application
SHALL be bought by removing chrome rather than by adding padding, because padding lowers the
rows visible on a report and an accountant comparing twelve periods then scrolls to compare.

#### Scenario: Both registers resolve the same token

- **WHEN** the landing page and an application screen both render the primary surface
- **THEN** both resolve the same token name
- **AND** neither declares its own value for it

#### Scenario: A display size does not appear in the application

- **WHEN** the application's components are inspected for type sizes
- **THEN** none uses a size above 24px

### Requirement: Both palettes ship, and their contrast is measured

The system SHALL provide a light and a dark palette, each chosen rather than derived by
inverting the other, and SHALL follow the reader's explicit choice where one has been made
and the system preference otherwise. Every foreground and background pair carrying text SHALL
meet WCAG AA at 4.5:1, measured rather than asserted.

The chrome SHALL be monochrome. Outside a chart's own frame, colour SHALL carry exactly one
job — state — and a state SHALL NEVER be communicated by colour alone: a word or an icon SHALL
always accompany it, because the report must print readably in black and white and because
a reader who cannot distinguish the hue must still be able to read the state.

Inside a chart, hue additionally carries series identity. That is the single exception, it is
bounded by the chart's frame, and its rules are a requirement of their own below.

#### Scenario: The reader's explicit choice wins over the system preference

- **WHEN** the system preference is dark and the reader has chosen light
- **THEN** the light palette renders

#### Scenario: A state is legible without its colour

- **WHEN** a rejected import batch is rendered in greyscale
- **THEN** its state is still identifiable from the word or the icon

#### Scenario: Printing produces a readable report

- **WHEN** a report is printed while the dark palette is active
- **THEN** the printed output uses a light ground and dark text

### Requirement: A figure is rendered from integer minor units and its currency

The system SHALL format every monetary figure from `int64` minor units and an ISO-4217 code,
SHALL NOT represent an amount as a JavaScript `number` at any point, and SHALL group and
punctuate according to the reader's locale. A negative figure SHALL be shown with a minus
sign, never with parentheses and never in red. A true zero SHALL render as a zero and a line
with no data SHALL render as a dash, because an accountant must be able to tell "nothing
happened" from "nothing was loaded".

The currency code SHALL appear once in a column header rather than in every cell, and every
numeric cell SHALL use tabular figures so that columns align by digit.

#### Scenario: An amount never becomes a JavaScript number

- **WHEN** the formatter receives minor units that exceed the safe integer range
- **THEN** the rendered figure is exact
- **AND** no intermediate value is a JavaScript `number`

#### Scenario: A non-base currency renders in its own denomination

- **WHEN** a figure carries a currency whose minor unit is not two decimal places
- **THEN** the rendered figure uses that currency's exponent
- **AND** it is not converted to the reporting currency without being labelled as converted

#### Scenario: A negative figure is not an error

- **WHEN** an expense line renders a negative amount
- **THEN** it carries a minus sign
- **AND** it is rendered in the ordinary text colour, not the danger colour

#### Scenario: Locale changes grouping without changing the value

- **WHEN** the reader switches from English to Russian
- **THEN** the grouping and decimal marks change
- **AND** the underlying minor units are unchanged

### Requirement: Tenant context comes from the session, never from the URL

The system SHALL read the organisation and the entity from the authenticated session and
SHALL NOT accept either from a URL path segment or a query parameter. A link SHALL identify
which figures to read; it SHALL NOT identify whose.

View state that names a figure — the report, the period range, the opened cell — SHALL live
in the URL so that a link reproduces what its sender saw, survives a reload and works with
the back button. The reader's language and theme SHALL NOT live in the URL: they belong to
the reader, not to the figure.

#### Scenario: A shared link does not carry a tenant identifier

- **WHEN** a drill-down link is copied
- **THEN** it carries the report, the cell and the period range
- **AND** it carries no organisation or entity identifier

#### Scenario: A recipient sees their own data or none

- **WHEN** a drill-down link is opened by a person outside the sending organisation
- **THEN** the tenant context resolves from their own session
- **AND** no figure belonging to the sender's organisation is rendered

#### Scenario: A link opens in the recipient's own language

- **WHEN** a link sent by a Russian-language reader is opened by an English-language reader
- **THEN** the interface renders in English
- **AND** the period range and the opened cell are unchanged

### Requirement: The public surface and the authenticated surface are separate

The system SHALL serve a public surface that requires no session and an authenticated surface
that requires one, and SHALL gate the authenticated surface as a whole rather than screen by
screen. A screen added to it SHALL be protected by having been added, not by remembering to
protect it.

The public surface SHALL NOT read application data. It SHALL NOT call an authenticated
endpoint, read the session, or load a screen's data at runtime. It MAY render the same
components the authenticated surface does, from content of its own: showing the real product
rather than a picture of it is what keeps a marketing page honest as the product changes.
The constraint is on reading state, not on sharing components — a marketing page that reaches
into application state cannot later be lifted into a static bundle without being rewritten.

The client SHALL NOT define a route under any path prefix served by `core`. Those prefixes
belong to the backend, and a client route claiming one shadows it in development, in
production, or in both.

#### Scenario: An added screen is protected by where it sits

- **WHEN** a new screen is added under the authenticated surface
- **THEN** an unauthenticated visitor to it is sent to sign in
- **AND** no per-screen guard had to be written for that to be true

#### Scenario: A completed sign-in lands where the session is resolved

- **WHEN** a person completes sign-in
- **THEN** they arrive on the authenticated surface, which resolves their session
- **AND** they do not arrive on the public surface, which cannot tell whether
  anyone is signed in

#### Scenario: A failed sign-in lands where its reason is rendered

- **WHEN** a sign-in fails and the browser is returned with an error code
- **THEN** it arrives on the screen that turns that code into a sentence
- **AND** the code is not left visible only in the URL

#### Scenario: The public page reads nothing that requires a session

- **WHEN** the public surface's imports are inspected
- **THEN** none resolves to a data module, a transport, or the session
- **AND** the page renders fully for a visitor with no session

#### Scenario: The client does not shadow a backend prefix

- **WHEN** the client's route table is compared with the prefixes the Ingress routes to `core`
- **THEN** no client route is defined under any of them

### Requirement: The client owns every sentence, and the backend owns none

The system SHALL render no user-facing sentence that came from the backend. The backend
returns codes; turning a code into a sentence is the client's work, and an unrecognised code
SHALL still render as something a person can read rather than as the code itself.

Every user-facing string SHALL be addressed by a key and SHALL exist in each supported
language. No sentence SHALL be written inline in a component, because a string that is not in
the catalogue is a string the second language silently lacks.

#### Scenario: An unknown error code still reads as a sentence

- **WHEN** the backend returns an error code the catalogue does not carry
- **THEN** a general readable message is shown
- **AND** the raw code is not presented as the message

#### Scenario: Both languages carry every key

- **WHEN** the message catalogue is checked
- **THEN** every key present in one language is present in the other

### Requirement: Screens render from a typed data layer, not from a transport

The system SHALL place every screen's data access behind a typed module whose shapes match
the contract the backend will carry — monetary amounts as integer minor-unit strings, states
as codes rather than sentences — so that connecting a real backend replaces a module body
and not a component.

Fixture data SHALL be confined to those modules. A component SHALL NOT hold sample data.

#### Scenario: A component does not import a transport

- **WHEN** a screen component is inspected for imports
- **THEN** it imports its data module
- **AND** it does not import a transport or a generated client directly

#### Scenario: Fixtures are internally consistent

- **WHEN** a report renders from fixtures
- **THEN** its totals, subtotals and percentages are computed from the same rows the table
  shows
- **AND** no total is a typed-in constant

### Requirement: Every asynchronous view has an empty, a loading and an error state

The system SHALL give every screen that reads data an empty state, a loading state and an
error state. An empty state SHALL name what is missing and SHALL NOT offer an action that is
not wired. These states SHALL be written as what a new customer sees rather than as
placeholders describing unbuilt work, because they are most of what is seen on a demonstration
day: the happy path is fast, and the empty path is where a new customer starts.

#### Scenario: An empty report names what is missing

- **WHEN** a report is opened for an organisation with no imported data
- **THEN** the screen states that no data has been imported
- **AND** it points at the screen where data is imported

#### Scenario: An unwired action is absent rather than inert

- **WHEN** an empty state is rendered for a capability whose backend does not exist
- **THEN** no disabled or non-functioning control is offered

### Requirement: A report names its basis and never guesses a blocked line

The system SHALL show the accounting basis of a report where it is read rather than in a
footnote, SHALL derive it from the source of the data rather than from a human choice, and
SHALL render a line whose sources are mixed and unmatched as blocked, naming the reason in
the same view.

A report SHALL show its reconciliation figures — opening, in, out, transfers, closing — and
SHALL show, for any figure opened, the transactions behind it with the category, the deciding
engine layer and the confidence. None of these SHALL be removable by a later visual
simplification: each is what makes a figure believable, and each is the first thing such a
pass deletes.

#### Scenario: A blocked line names its reason

- **WHEN** a report line draws on both ledger and bank sources with no confirmed match
- **THEN** the line renders as blocked rather than as a computed figure
- **AND** the reason is stated in the same view

#### Scenario: The basis is visible where the figures are

- **WHEN** a report is read
- **THEN** the basis is shown with the table
- **AND** it is not placed in a footnote or behind a control

### Requirement: The interface is usable from the keyboard

The system SHALL render a visible focus indicator on every interactive element, SHALL NOT
convey information on hover alone, and SHALL provide a persistently visible key legend on any
screen whose primary interaction is the keyboard. A legend placed behind a help control is
not visible.

The system SHALL honour a reader's reduced-motion preference, SHALL restrict motion in the
application to overlays, and SHALL NEVER animate a figure: a number that counts up reads as
still computing, and a report's whole claim is that its figures are final.

#### Scenario: Focus is visible on every control

- **WHEN** the interface is traversed by keyboard
- **THEN** each focused element shows a focus indicator
- **AND** the indicator is distinguishable from the element's own fill

#### Scenario: A figure does not animate

- **WHEN** a report renders or re-renders its figures
- **THEN** no figure transitions between values

#### Scenario: Reduced motion is honoured

- **WHEN** the reader's system requests reduced motion
- **THEN** transitions and animations are suppressed

### Requirement: A chart's colour is validated, and hue carries identity only inside it

The system SHALL confine hue-as-identity to a chart's own frame. Outside one, colour SHALL
mean state and nothing else. A status colour SHALL NEVER be used as a series colour, and a
series colour SHALL NEVER be presented as a state.

Every categorical palette SHALL be validated rather than chosen: each slot within the mode's
lightness band and above the chroma floor, adjacent pairs separated under simulated
protanopia and deuteranopia, and contrast measured against the surface the chart actually
renders on, in both themes. Slot order SHALL be fixed and SHALL NOT be cycled or reassigned
when a filter changes the number of series, because a colour that follows rank rather than
identity repaints the survivors and silently changes what the reader is comparing.

A ninth category SHALL fold into a single "Other" rather than take a generated hue.

No chart SHALL carry two vertical scales. Two measures of different magnitude SHALL become
two charts, small multiples, or a common index.

A series SHALL be identifiable without colour: a legend SHALL be present wherever two or more
series appear, and every chart SHALL offer a table view of the same figures. Text SHALL wear
text colours and never a series colour.

A sequential encoding SHALL use one hue in ordered lightness steps. A diverging encoding SHALL
use two hues with a neutral midpoint, and SHALL NOT use red for the negative arm: a loss is
not an error, and red is reserved for a failed import.

#### Scenario: A palette slot is not chosen by eye

- **WHEN** a categorical palette is proposed or changed
- **THEN** every slot is checked against the lightness band, the chroma floor, adjacent
  colourblind separation and contrast against its own surface
- **AND** a slot that fails is re-stepped before it ships

#### Scenario: Filtering does not repaint the remaining series

- **WHEN** a chart with five series is filtered down to three
- **THEN** each remaining series keeps the colour it had
- **AND** no series takes a colour that belonged to a removed one

#### Scenario: A negative column is not red

- **WHEN** a month with a negative net result renders in a diverging column chart
- **THEN** the column takes the negative arm's hue
- **AND** that hue is not the colour used to mark a rejected import

#### Scenario: A series is identifiable without colour

- **WHEN** a chart with two or more series is viewed in greyscale
- **THEN** a legend and either direct labels or the table view identify each series

#### Scenario: The ninth category does not invent a colour

- **WHEN** a chart receives more categories than the palette has slots
- **THEN** the smallest are folded into a single "Other"
- **AND** no colour outside the documented slots is generated

### Requirement: A headline figure is a number, not a chart

The system SHALL present the report's headline measures — revenue, expenses, net result and
the unreviewed amount — as figures rather than as charts, and SHALL compute them from the
same rows the report renders so that a headline cannot disagree with the table beneath it.

A single value SHALL NOT be drawn as a one-bar chart.

#### Scenario: A headline agrees with the table

- **WHEN** the headline figures and the report table are rendered from the same data
- **THEN** each headline equals the corresponding total in the table

#### Scenario: The unreviewed amount is stated as money

- **WHEN** items await review
- **THEN** the headline states the amount awaiting review, not only a count
