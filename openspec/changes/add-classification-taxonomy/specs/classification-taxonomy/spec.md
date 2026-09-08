# classification-taxonomy Specification

## Purpose

The list of categories a transaction can be classified into, and the report lines those
categories become. Shared where accounting structure is shared, per-organisation where a
business is particular, and versioned so that a change made in June does not redraw March.

## Requirements

### Requirement: A category that a shared rule targets is itself shared

The system SHALL make a category shared whenever a rule belonging to no organisation points
at it, together with every ancestor of such a category and the report sections themselves.
Everything else SHALL belong to an organisation.

This is forced rather than chosen. A template rule has no `org_id`, and every organisation
holds its own identifier for its own copy of a per-organisation category, so one rule cannot
name them all. Depth is not the test: fourteen of the nineteen categories the templates
target sit three to five levels down, and every one of them — bank commission, VAT, currency
exchange, office rent — is universal rather than particular to a business.

#### Scenario: Every rule resolves against the shared tree alone

- **WHEN** the category codes named by the template rules are compared against the shared
  categories
- **THEN** every one of them is present
- **AND** none of them belongs to an organisation

#### Scenario: A category no rule targets is not shared

- **WHEN** a category exists that no template rule targets and that is not an ancestor of one
  or a report section
- **THEN** it is not seeded as shared
- **AND** it appears in the industry template instead, to be copied by the organisation that
  adopts it

### Requirement: A single versioned category tree, shared and tenant in one table

The system SHALL store categories in one table where a row is either shared by every
organisation or owned by exactly one. A shared row SHALL have a NULL `org_id` and
`scope = 'global'`; an owned row SHALL have both populated. Every row SHALL carry a
`taxonomy_version`.

#### Scenario: An organisation sees the shared tree plus its own leaves

- **WHEN** a member of organisation A reads the taxonomy for a given `taxonomy_version`
- **THEN** every shared row is returned
- **AND** every row owned by A is returned
- **AND** no row owned by any other organisation is returned

#### Scenario: A leaf may hang under a shared parent

- **WHEN** organisation A creates a category whose parent is a shared row
- **THEN** the row is created
- **AND** it is visible to A and to no other organisation

#### Scenario: A row cannot claim an owner it does not have

- **WHEN** a row is written with `scope = 'global'` and a populated `org_id`, or with
  `scope = 'org'` and a NULL `org_id`
- **THEN** the write is rejected by a check constraint

### Requirement: The application can read shared rows but never write them

The system SHALL grant `vekst_app` read access to shared rows and no write access to them.
Shared rows SHALL be created, altered and removed only by migration. The read policy and
the write policy SHALL be separate policies, so that admitting a shared row to a reader does
not admit it to a writer.

#### Scenario: A shared row cannot be modified through the application

- **WHEN** the application attempts to update the formula of a shared row while acting for
  organisation A
- **THEN** the statement affects no row
- **AND** the stored formula is unchanged for every organisation

#### Scenario: A shared row cannot be created through the application

- **WHEN** the application attempts to insert a row with a NULL `org_id`
- **THEN** the insert is rejected by the write policy

### Requirement: Categories are isolated between organisations

The system SHALL apply row-level security to `categories`, with `FORCE ROW LEVEL SECURITY`,
so that the table owner is subject to the policy as well.

#### Scenario: One organisation cannot read another's categories

- **WHEN** the tenant context is organisation A and a query selects every row
- **THEN** no row owned by organisation B appears in the result

#### Scenario: One organisation cannot write another's categories

- **WHEN** the tenant context is organisation A and a statement updates or deletes a row
  owned by B
- **THEN** no row is affected
- **AND** the outcome is indistinguishable from the row not existing

#### Scenario: A parent in another organisation is not an existence oracle

- **WHEN** organisation A creates a category naming a parent owned by organisation B
- **THEN** the write is rejected
- **AND** the error is identical to the error raised when the named parent does not exist at
  all

### Requirement: Computed lines exist but can never receive a transaction

The system SHALL mark GM, NM, CM, IBT and NI as computed, store the formula that produces
each, and exclude every computed row from the set a classification may target.

#### Scenario: A computed line is never offered as a classification target

- **WHEN** the classifiable categories are read
- **THEN** no row marked computed is returned
- **AND** no row that is not a leaf is returned

#### Scenario: A computed row must carry its formula

- **WHEN** a row is written as computed with a NULL formula, or as both computed and a leaf
- **THEN** the write is rejected by a check constraint

### Requirement: Unallocated payroll is a state, not a guess

The system SHALL mark the payroll and payroll-tax buckets as requiring allocation, so a
report can show the amount that is known to be payroll but not yet attributed to a
department, rather than attributing it to one.

#### Scenario: The payroll buckets are marked

- **WHEN** the taxonomy is read
- **THEN** exactly the two payroll buckets carry `requires_allocation`
- **AND** no other row carries it

### Requirement: The seeded taxonomy matches its generator

The system SHALL seed the shared rows from `eval/out/seed_categories.sql`, and CI SHALL fail
when re-running `eval/emit.py` produces a diff, so the committed seed and the generator
cannot disagree.

#### Scenario: Regenerating the seed produces no diff

- **WHEN** CI runs `eval/emit.py` on a pull request
- **THEN** `git diff --exit-code` reports no change

#### Scenario: Every rule points at a category that exists

- **WHEN** the category codes referenced by `eval/out/seed_rules.sql` are compared against
  the codes in `eval/out/seed_categories.sql`
- **THEN** every referenced code is present

#### Scenario: The seed applies to an empty database

- **WHEN** migration 005 is applied to a database containing only the preceding migrations
- **THEN** the shared rows are inserted before row-level security is enabled and forced
- **AND** the migration succeeds as `vekst_migrator` without that role being a superuser
