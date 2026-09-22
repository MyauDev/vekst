# identity-access Specification

## Purpose
TBD - created by archiving change add-identity. Update Purpose after archive.
## Requirements
### Requirement: A person signs in with Google

The system SHALL authenticate people through Google as an OpenID Connect provider, using the
authorisation-code flow with PKCE. The flow SHALL be served on plain HTTP routes rather than
as remote procedure calls, because it is a browser redirect. The system SHALL validate the
returned ID token's signature, issuer, audience, expiry and nonce before trusting any claim
in it.

#### Scenario: A first sign-in creates an account

- **WHEN** a person completes the Google flow and no identity matches the returned subject
- **THEN** a user record and an identity record are created
- **AND** the person is signed in

#### Scenario: A later sign-in reuses the account

- **WHEN** the same person signs in again
- **THEN** the existing user is used
- **AND** no second user record is created

#### Scenario: An unverified email is refused

- **WHEN** the provider reports the email as unverified
- **THEN** the sign-in is refused
- **AND** no user, identity or session is created

#### Scenario: A token that does not validate is refused

- **WHEN** the returned ID token has an invalid signature, an audience other than this
  client, an expiry in the past, or a nonce that does not match the flow
- **THEN** the sign-in is refused in each case
- **AND** the response carries an error code, not a sentence

#### Scenario: A callback that matches no flow is refused

- **WHEN** the callback arrives with a state value matching no pending flow, or one already
  consumed
- **THEN** the sign-in is refused
- **AND** no session is created

#### Scenario: A callback must reach the browser that started the flow

- **WHEN** a callback carries a state value that does match a pending flow, but arrives without
  the flow's browser-bound value, or carries one naming a different flow
- **THEN** the sign-in is refused
- **AND** the refusal is indistinguishable from a state value that matched nothing

#### Scenario: A completed flow cannot be replayed

- **WHEN** a callback that has already been completed is presented a second time
- **THEN** the sign-in is refused
- **AND** the pending flow no longer exists to be matched

### Requirement: An account is identified by provider and subject, never by email

The system SHALL resolve a sign-in by the pair of provider and provider subject. An email
address SHALL be descriptive only, and SHALL NOT be used to match a sign-in to an existing
account.

#### Scenario: A changed email still reaches the same account

- **WHEN** a person whose provider email has changed since their last sign-in signs in again
- **THEN** the same account is used, because the subject is unchanged
- **AND** the stored address is left as it was

#### Scenario: An email already held by another account is refused

- **WHEN** a sign-in returns a new subject whose email already belongs to a different user
- **THEN** the sign-in is refused with a code
- **AND** the two accounts are not merged
- **AND** the refusal comes from the stored constraint rather than a prior read, so two
  simultaneous first sign-ins on one address cannot both succeed

#### Scenario: A returning person is never locked out by another account's address

- **WHEN** a person whose subject is already known signs in, and the address their provider now
  reports belongs to a different account
- **THEN** the sign-in succeeds
- **AND** no account record is written

### Requirement: An account record is provisioned once and never written by a claim

The system SHALL write the account record when the account is created, from the claims of the
identity that creates it, and SHALL NOT update it on any later sign-in. The account record
SHALL change afterwards only by a deliberate act. This is what allows one person to hold
several sign-in methods without those methods overwriting one another.

#### Scenario: A later sign-in writes nothing to the account

- **WHEN** a person with a known identity signs in
- **THEN** the account record is unchanged
- **AND** the person is signed in

#### Scenario: An account may exist without an address

- **WHEN** a provider asserts no email address
- **THEN** an account can still be provisioned
- **AND** the absence does not collide with another account that also has none

#### Scenario: A provider's language tag is normalised before it is stored

- **WHEN** a provider reports a regional language tag such as `en-GB`, or one the product does
  not support
- **THEN** the account is provisioned with a supported language, falling back to the default
- **AND** the sign-in is not refused because of the tag

### Requirement: The binding between an identity and an account is immutable

The system SHALL create the binding between a provider subject and an account once, and SHALL
NOT permit it to be repointed at a different account. The prohibition SHALL be enforced by the
database, not by application code, because a repointed binding hands the holder of that
provider subject every organisation the original account belongs to.

#### Scenario: A binding cannot be moved to another account

- **WHEN** an attempt is made to change which account a provider subject resolves to
- **THEN** the attempt fails at the database
- **AND** it fails whether or not the application code intended it

### Requirement: A deployment without sign-in configured still serves

The system SHALL treat absent provider credentials as a supported state: it SHALL start, serve
every other route, and answer the sign-in routes with a configuration code. It SHALL refuse to
start only when a credential is present but is a placeholder value, because such a value can
only mean a configuration step was begun and left unfinished, and it fails opaquely at the
provider rather than at the point of the mistake.

#### Scenario: Sign-in routes answer when no credentials are configured

- **WHEN** the sign-in route is requested and no provider credentials are configured
- **THEN** it answers with a configuration error code
- **AND** the rest of the system serves normally

#### Scenario: A placeholder credential stops the system starting

- **WHEN** a provider credential is present but holds the placeholder value
- **THEN** startup fails, naming the setting at fault

### Requirement: A session is server-side, opaque and revocable

The system SHALL keep session state in the database and SHALL give the browser an opaque
random token in an `HttpOnly` cookie. The system SHALL store only a cryptographic hash of
that token. A session SHALL be revocable, and a revoked or expired session SHALL grant
nothing.

#### Scenario: The stored session cannot be used to sign in

- **WHEN** the sessions table is read
- **THEN** it contains a hash of the cookie value
- **AND** it contains no value that a browser could present as a session

#### Scenario: Signing out ends the session immediately

- **WHEN** a person signs out and the same cookie is presented again
- **THEN** the request is unauthenticated
- **AND** the session is marked revoked rather than deleted silently

#### Scenario: An expired session grants nothing

- **WHEN** a cookie is presented whose session is past its expiry
- **THEN** the request is unauthenticated

#### Scenario: A session is created only after a successful exchange

- **WHEN** the code exchange or token validation fails at any step
- **THEN** no session exists for that attempt

### Requirement: Authenticated routes require a session; health does not

The system SHALL reject a call to an authenticated route that carries no valid session, with
an unauthenticated error code. Liveness, readiness and the sign-in routes themselves SHALL
remain reachable without one.

#### Scenario: An anonymous call is rejected

- **WHEN** an authenticated remote procedure call arrives with no cookie, an unknown cookie,
  or a revoked one
- **THEN** it is rejected with the unauthenticated code in each case
- **AND** the response body carries no user data

#### Scenario: Probes still answer

- **WHEN** liveness and readiness are requested with no cookie
- **THEN** both answer as before
- **AND** neither carries user data

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

### Requirement: Sign-in flows and sessions expire

The system SHALL expire pending sign-in flows and old sessions, and SHALL remove them with a
background job rather than leaving them to accumulate.

#### Scenario: Abandoned flows are removed

- **WHEN** a person starts a sign-in and never completes it, and the expiry job runs after
  the flow's lifetime
- **THEN** the pending flow is gone
- **AND** flows still within their lifetime remain

#### Scenario: Expired sessions are removed

- **WHEN** the expiry job runs after a session's retention window
- **THEN** that session row is gone
- **AND** live sessions remain

