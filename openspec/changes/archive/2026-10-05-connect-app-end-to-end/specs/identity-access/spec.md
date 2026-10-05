# identity-access Specification — delta

## MODIFIED Requirements

### Requirement: The current-user call answers which organisations the caller belongs to

The system SHALL return, with the signed-in user, every organisation the caller holds a
membership in, each with its name, base currency, the caller's own role, and its
entities. An empty list SHALL be a successful answer and never an error.

`User` itself is unchanged. The addition is a second field on the response, because the
message `add-identity` shipped said in its own comment that change 1.1 would extend it,
and 1.1 did not. Until it does, the browser holds no organisation identifier and cannot
make a single tenant-scoped call — which is the state the product is in.

An empty list is the first-run signal. Reporting it as an error would make "has not yet
created an organisation" indistinguishable from "something went wrong", and the browser
would have to guess which one to render.

The read goes through memberships under row-level security for the identifiers, then
fetches organisations and entities by key. It SHALL NOT join `users`.

#### Scenario: A member receives their organisation and its entities

- **WHEN** a signed-in caller holds an owner membership in one organisation
- **THEN** the response carries that organisation, its base currency, the role `owner`,
  and the organisation's single entity

#### Scenario: A first-time caller receives an empty list

- **WHEN** a signed-in caller holds no membership
- **THEN** the call succeeds
- **AND** the organisations list is empty
- **AND** no error code is returned

#### Scenario: Another organisation is absent, not hidden by the client

- **WHEN** organisation A and organisation B both exist
- **AND** the caller belongs to A only
- **THEN** the response carries A alone
- **AND** B is absent from the response rather than filtered after it arrives
