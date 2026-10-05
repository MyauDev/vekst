# tenancy Specification — delta

## ADDED Requirements

### Requirement: A signed-in person can create their organisation

The system SHALL expose exactly one browser-facing call that creates an organisation, its
first entity and the caller's owner membership, and that call SHALL be the only one a
caller holding a session but no membership may make.

Every other call resolves an organisation from a membership the caller already holds. A
person who has just signed in for the first time holds none, so without this call the
product has no first minute. Containment is unchanged: the identifier is minted inside
`core/internal/db` before the transaction opens, so the insert satisfies
`organizations`' own policy rather than escaping it.

#### Scenario: A first-time caller creates an organisation

- **WHEN** a caller with a valid session and no membership calls `CreateOrganization`
  with a name, a supported country, a supported base currency and an entity name
- **THEN** an organisation, one entity and one owner membership are written in one
  transaction
- **AND** the response carries the organisation identifier and the entity identifier

#### Scenario: A caller who already belongs to an organisation is refused

- **WHEN** a caller holding any membership calls `CreateOrganization`
- **THEN** the call is refused with the code `already_a_member`
- **AND** no organisation, entity or membership row is written

#### Scenario: An unsupported currency is refused before anything is written

- **WHEN** the request names a currency that `core/internal/money`'s exponent map does
  not hold
- **THEN** the call is refused with `unsupported_currency`
- **AND** no row is written
- **AND** the refusal does not rely on a foreign key: `organizations.base_currency`
  checks only the three-letter shape and carries no reference to `currencies` (below), so
  `XXX` reaches the table unless Go refuses it first

### Requirement: Creating an organisation adopts the industry template

The system SHALL copy every `category_templates` row into the new organisation as a
`scope = 'org'` category, inside the same transaction that creates the organisation, and
SHALL resolve each row's parent as that organisation's own category with the parent code,
failing that the shared category with the parent code.

Migration 005 seeds the shared level-1 and level-2 nodes only. An organisation without
the 60 leaves can be classified and reported against, but every figure lands on a
top-level section, which is a P&L nobody asked for. Adoption at creation is the only
moment at which every organisation gets the same taxonomy — adopting later makes an
organisation's taxonomy depend on when it first imported a file.

#### Scenario: A leaf under a shared parent

- **WHEN** a template row's parent code names a shared category
- **THEN** the adopted row's `parent_id` is that shared category
- **AND** the row's own `org_id` is the new organisation

#### Scenario: A leaf under the organisation's own parent

- **WHEN** a template row's parent code names another template row
- **THEN** the adopted row's `parent_id` is that organisation's copy, never the template
  table's row

#### Scenario: An unresolvable parent leaves nothing behind

- **WHEN** a template row's parent code matches no shared category and no adopted row
- **THEN** the transaction fails
- **AND** no organisation, entity, membership or category exists afterwards

### Requirement: The industry template holds no tenant data

The system SHALL keep `category_templates` free of any organisation identifier, SHALL
grant `vekst_app` `SELECT` on it and nothing else, and SHALL record it in
`deploy/db/rls-exempt-tables.txt` as a table that holds no tenant row.

A table every customer reads and no customer owns is the one shape the RLS coverage test
cannot classify on its own. Saying so in the allowlist is a security decision taken
deliberately, with two reviewers, rather than an exception discovered later.

#### Scenario: The coverage test refuses a silent exemption

- **WHEN** the allowlist line for `category_templates` is removed
- **THEN** the RLS coverage test fails

#### Scenario: One organisation cannot see another's adopted categories

- **WHEN** organisation A and organisation B have both adopted the template
- **AND** a transaction is bound to B
- **THEN** none of A's adopted categories is readable, by list or by identifier

### Requirement: The currency reference table holds no tenant data

The system SHALL keep `currencies` free of any organisation identifier, SHALL grant
`vekst_app` `SELECT` on it and nothing else, and SHALL record it in
`deploy/db/rls-exempt-tables.txt` as a table that holds no tenant row.

The table is seeded from `core/internal/money`'s exponent map, the codebase's existing
canonical currency data. A copy that drifts from its source is worse than no table at
all: a code Go accepts and the schema has no record of, or the reverse, is exactly the
inconsistency this table exists to remove.

#### Scenario: The coverage test refuses a silent exemption

- **WHEN** the allowlist line for `currencies` is removed
- **THEN** the RLS coverage test fails

#### Scenario: The seed matches its source

- **WHEN** the migration's seed rows for `currencies` are compared against
  `core/internal/money`'s exponent map
- **THEN** every code and every exponent agree
- **AND** the comparison is a checked build step, not a manual review
