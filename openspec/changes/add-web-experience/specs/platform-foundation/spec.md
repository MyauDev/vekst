## MODIFIED Requirements

### Requirement: The browser reaches the API same-origin through the Ingress

The system SHALL route browser traffic through a single Ingress host, sending `/rpc/*` and
`/auth/*` to the `core` Service and all other paths to the `web` Service. No cross-origin
request SHALL be required, and the routing SHALL be defined in the Kustomize base so that
every environment inherits it.

`/auth/*` SHALL be routed to `core` for the same reason `/rpc/*` is, and separately from it.
The sign-in flow is three plain HTTP routes rather than Connect calls because a Connect
handler cannot answer with a 302 and the callback is a browser navigation; that makes them
part of `core`'s surface while looking, to a path-matching rule, like application paths. A
routing rule that sends everything except `/rpc` to the static file server therefore answers
the sign-in flow with the application's `index.html`, and sign-in fails in every environment
while the manifest looks correct.

The development server SHALL proxy the same two prefixes to `core`, so that a bare `vite dev`
outside the cluster and the cluster itself agree about which paths belong to which service.
A prefix routed in one and not the other produces a flow that works in exactly one
environment, which is the shape of defect that survives review.

#### Scenario: RPC and application share one origin

- **WHEN** the browser loads the application and issues a `HealthService/Check` call
- **THEN** both the document and the RPC are served from the same Ingress host
- **AND** the RPC succeeds with no cross-origin response header

#### Scenario: The sign-in flow reaches core rather than the file server

- **WHEN** the browser navigates to `/auth/google/start` through the Ingress
- **THEN** the request is answered by the `core` Service
- **AND** it is not answered by the `web` Service with the application document

#### Scenario: The development proxy and the Ingress agree

- **WHEN** the prefixes the development server proxies to `core` are compared with the
  Ingress paths routed to the `core` Service
- **THEN** both carry `/rpc` and `/auth`
- **AND** neither carries a prefix the other does not

#### Scenario: Routing is not redefined per environment

- **WHEN** the `local` overlay is compared to the base
- **THEN** the overlay changes the ingress host only
- **AND** it does not restate the path routing rules
