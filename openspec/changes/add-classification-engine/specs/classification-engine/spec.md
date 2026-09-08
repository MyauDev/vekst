# classification-engine Specification

## Purpose

Given a batch of transactions and the organisation's classification context, propose a
category for each. A proposal, never a posting: a machine does not decide what a company's
accounts say.

## Requirements

### Requirement: The engine is a pure function of its request

The system SHALL implement `vekst.internal.v1.ClassifierService/ClassifyBatch` such that the
response depends only on the request. The classifier SHALL hold no database handle, read no
clock it was not given, and keep no state between calls.

#### Scenario: The same request twice gives the same answer

- **WHEN** an identical `ClassifyBatchRequest` is sent twice, in either order, to any
  instance
- **THEN** both responses are byte-identical

#### Scenario: The classifier has no route to the database

- **WHEN** the `classifier` container and its configuration are inspected in every overlay
- **THEN** no database host, port, user, password or connection string is present
- **AND** the image contains no Postgres client library

### Requirement: Layers are tried in order and the first answer wins

The system SHALL evaluate vendor memory, then a regulated code carried by the source, then
rules, and SHALL return the first layer that answers. Each proposal SHALL name the layer that
produced it and the evidence it rested on.

#### Scenario: Vendor memory outranks everything

- **WHEN** a transaction matches vendor memory, a regulated code and a text rule at once
- **THEN** the proposal names layer `L0`
- **AND** its evidence names the tier of the counterparty key that matched

#### Scenario: A regulated code outranks a text rule

- **WHEN** a transaction carries a regulated code with a rule for it, and also matches a text
  rule of lower priority
- **THEN** the proposal names layer `L0.5`

#### Scenario: Nothing answers

- **WHEN** no layer matches a transaction
- **THEN** no proposal is returned for it
- **AND** the response is not an error

### Requirement: A regulated code is trusted by its issuer, not by its source kind

The system SHALL apply the regulated-code layer to any transaction carrying such a code,
whether it came from a ledger or from a bank, and SHALL record which issuer assigned it.

#### Scenario: A Kazakh bank row is classified by its state payment-purpose code

- **WHEN** a bank transaction carries КНП `911` in the debit direction
- **THEN** the proposal names layer `L0.5`
- **AND** the category is the payroll-tax bucket

#### Scenario: A rule scoped to one source kind does not fire on the other

- **WHEN** a rule is scoped to ledger rows and a bank transaction otherwise matches every
  condition
- **THEN** the rule does not fire

### Requirement: Rules are ordered, and specific beats general

The system SHALL evaluate rules in priority order and stop at the first whose every
condition holds. Priority SHALL be unique within what one organisation sees, so that no
answer depends on row order.

#### Scenario: A two-part rule beats its own prefix

- **WHEN** a transaction's description contains both `ПОДОХОДНЫЙ НАЛОГ` and `ИЗ ДИВИДЕНДОВ`,
  and rules exist for the pair and for the first part alone
- **THEN** the pair wins
- **AND** the category is outside the P&L rather than a payroll tax

#### Scenario: Duplicate priorities are rejected

- **WHEN** two rules visible to one organisation are stored with the same priority for one
  taxonomy and ruleset version
- **THEN** the write is rejected by a unique index

### Requirement: Template rules belong to no organisation

The system SHALL store a rule with a NULL `org_id` when it belongs to a country or a bank
rather than to a customer, SHALL let every organisation read such rules, and SHALL let none
of them write one.

#### Scenario: An organisation sees the templates and its own rules

- **WHEN** the effective rules for organisation A are read
- **THEN** every template rule is returned
- **AND** every rule owned by A is returned
- **AND** no rule owned by another organisation is returned

#### Scenario: An organisation cannot alter a template rule

- **WHEN** the application, acting for organisation A, attempts to update or delete a rule
  with a NULL `org_id`
- **THEN** no row is affected

### Requirement: Vendor memory is isolated per organisation

The system SHALL scope vendor memory to one organisation, with row-level security and
`FORCE ROW LEVEL SECURITY`, and SHALL record the version of the key function that produced
each key.

#### Scenario: Memory does not cross a tenant boundary

- **WHEN** organisation A classifies a counterparty that organisation B has already approved
- **THEN** A's proposal does not come from B's memory
- **AND** the transaction reaches a lower layer or no layer at all

### Requirement: Normalisation is applied before the request and asserted within it

The system SHALL normalise descriptions and counterparty keys in `core`, SHALL state the
version used in the request, and the classifier SHALL refuse a version it does not implement.

#### Scenario: A version the engine does not implement is refused

- **WHEN** a request declares a `normalize_version` the classifier does not implement
- **THEN** the call fails with a specific error code
- **AND** no proposal is returned

#### Scenario: The Go and reference implementations agree

- **WHEN** the Go normaliser and the Python reference are run over the committed fixtures
- **THEN** every output is identical

### Requirement: The engine never proposes a category it was not given

The system SHALL return only category codes present in the request, and the request SHALL
carry only classifiable leaves.

#### Scenario: A computed line is never proposed

- **WHEN** a batch is classified
- **THEN** no proposal names a computed line such as GM, NM, CM, IBT or NI

#### Scenario: An unknown category is refused rather than passed through

- **WHEN** a rule in the request names a category code absent from the request's category
  list
- **THEN** the call fails
- **AND** no partial set of proposals is returned

### Requirement: Money never becomes a float

The system SHALL carry every amount as `vekst.type.v1.Money` — minor units and an ISO-4217
code — in the contract, in the matcher and in every generated type.

#### Scenario: An amount rule works in a currency with a different exponent

- **WHEN** an amount-range rule is evaluated against a JPY amount, whose exponent is 0, and a
  KWD amount, whose exponent is 3
- **THEN** both comparisons use minor units and neither rounds

#### Scenario: Comparing two currencies is refused

- **WHEN** an amount condition compares amounts with different currency codes
- **THEN** the comparison fails rather than returning a result

#### Scenario: No money field is floating point

- **WHEN** the generated Python and Go types are inspected by the no-float guard
- **THEN** no field carrying an amount is a floating-point type
