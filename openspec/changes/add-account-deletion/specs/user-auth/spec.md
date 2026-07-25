## MODIFIED Requirements

### Requirement: Stateless cookie session

The system SHALL issue stateless JWTs (HS256) on register and login, delivered
in an httpOnly cookie, and SHALL validate that cookie on protected requests.

- The token SHALL encode the user id as its subject and carry an expiry.
- The cookie SHALL be `HttpOnly` and `SameSite=Lax`, with `Secure` configurable
  (set in HTTPS deployments) and a max-age matching the token expiry.
- A protected handler MUST be able to resolve the authenticated user's id from
  the validated cookie.
- A cryptographically valid token SHALL NOT by itself authenticate a request:
  the subject MUST still resolve to an existing account. A token whose subject
  no longer exists SHALL be treated as unauthenticated.

#### Scenario: Valid cookie grants access

- **WHEN** a client calls a protected endpoint with a valid, unexpired session cookie
- **THEN** the system resolves the user from the cookie and serves the request

#### Scenario: Missing cookie

- **WHEN** a client calls a protected endpoint with no session cookie
- **THEN** the system responds `401` and does not serve the protected resource

#### Scenario: Expired or invalid signature

- **WHEN** a client calls a protected endpoint with an expired cookie or one whose signature does not verify against the server secret
- **THEN** the system responds `401`

#### Scenario: Token for a deleted account

- **WHEN** a client calls a protected endpoint with an unexpired, correctly signed cookie whose subject is an account that has been deleted
- **THEN** the system responds `401`, rather than admitting the request and failing later on a missing user
