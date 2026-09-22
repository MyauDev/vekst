# web-app Specification — delta

## ADDED Requirements

### Requirement: A signed-in person with no organisation is asked to create one

The system SHALL render a first-run screen, in place of the application shell's children,
when the current-user call returns an empty organisations list, and SHALL make no
tenant-scoped data call until an organisation exists.

The gate belongs in the shell for the same reason the sign-in gate does: a screen added
under the shell is covered by having been added, not by somebody remembering. First-run
is a state of the shell and not a route — a route would be reachable by a person who
already has an organisation.

#### Scenario: The first-run screen replaces the shell's children

- **WHEN** the current-user call succeeds with an empty organisations list
- **THEN** the first-run screen renders
- **AND** no import, review or report call is made

#### Scenario: Creating an organisation reveals the application

- **WHEN** the first-run form is submitted and the call succeeds
- **THEN** the current-user query is invalidated and refetched
- **AND** the application shell renders with the new organisation and entity named in the
  top bar

### Requirement: The organisation on screen is held in one place

The system SHALL hold the current organisation and entity identifiers in a single module,
set once by the application shell after the current-user call resolves, and SHALL read
them there in the data modules rather than passing them through components.

`add-web-experience` design D1 requires that connecting a real backend replaces a module
body and touches no component, and a test enforces that no screen imports a transport.
Threading an identifier through every screen would break both for a value no screen
displays.

#### Scenario: A data call before the session is set is a programming error

- **WHEN** a data module is called before the shell has set the session
- **THEN** it throws a coded error
- **AND** it does not send an empty organisation identifier to the server

#### Scenario: No screen imports a transport

- **WHEN** the seam test inspects every screen module
- **THEN** none imports a transport or a generated client
- **AND** the test is unchanged from before this change

## MODIFIED Requirements

### Requirement: Every figure on screen is computed by the pipeline

The system SHALL read the imports list, the review queue and the management P&L from
core over Connect, and SHALL NOT compute any of them in the browser from fixture data.
The view interfaces the data modules export SHALL keep their shape, so no component
changes.

`IMPLEMENTATION_PLAN.md` §3 defines not-done as a demo on invented data. Three of the
four screens are there today: `ReportService` and `ReviewService` are served and never
called. Keeping the exported shapes is what proves the seam did its job; a diff that
reaches a component is a sign it did not.

Money stays an `int64` minor-unit string and an ISO-4217 code across the new boundary.
The browser is where a right number becomes a wrong one.

#### Scenario: The P&L is the pipeline's answer

- **WHEN** the report screen loads for an entity with imported and classified rows
- **THEN** every figure comes from `GetManagementPNL`
- **AND** no fixture array remains in `web/src/data/report.ts`

#### Scenario: A drill-down opens the transactions behind the figure

- **WHEN** a figure is opened
- **THEN** its rows come from `ListLineTransactions` for the same entity, basis, period
  and line

#### Scenario: The review picker offers only classifiable categories

- **WHEN** the review screen's category picker loads
- **THEN** its options come from `ListCategories`
- **AND** no computed line and no non-leaf node is offered

#### Scenario: A report with no basis is never requested

- **WHEN** the report screen builds a request
- **THEN** it names a basis
- **AND** `REPORT_BASIS_UNSPECIFIED` is never sent

#### Scenario: A minor-unit amount is never a JavaScript number

- **WHEN** a money field crosses any rewritten data module
- **THEN** it is carried as a string of minor units with its currency code
- **AND** a test fails if the field's generated type is a `number`
