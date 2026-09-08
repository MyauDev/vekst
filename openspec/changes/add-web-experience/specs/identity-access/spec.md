## MODIFIED Requirements

### Requirement: Signing in grants no access to data

The system SHALL NOT imply any organisation membership or permission from the fact of being
signed in. Being a member of an organisation SHALL NOT by itself confer a right to any
operation within it.

The current-user response MAY carry the organisation and entity the session resolved, and
SHALL NOT carry a role. Naming the tenant a session already belongs to is not a grant: the
browser needs it to label what the reader is looking at, and it is the same value the server
would use whether or not it were sent. A role is different — a role is an assertion about what
may be done, nothing enforces one yet, and a field the front end can read is a field it starts
trusting.

The organisation and entity SHALL be resolved from the session on the server. They SHALL NOT
be accepted from the client, in a request field, a path segment or a query parameter, because
a tenant identifier the caller supplies is a tenant identifier the caller chose.

This requirement previously stated that the response carried no organisation at all, which was
correct while no tenancy model existed. `add-tenancy-and-rls` created one.

#### Scenario: The current user carries its organisation but no role

- **WHEN** a signed-in person requests the current user
- **THEN** the response identifies the person
- **AND** it carries the organisation and entity resolved from their session
- **AND** it contains no role field

#### Scenario: A supplied organisation is not honoured

- **WHEN** a request carries an organisation identifier the caller supplied
- **THEN** the tenant context is resolved from the session instead
- **AND** the supplied value changes nothing about which data is reachable

#### Scenario: Membership alone authorises nothing

- **WHEN** a signed-in member of an organisation attempts an operation that a role would gate
- **THEN** membership by itself is not treated as permission to perform it
