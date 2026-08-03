## MODIFIED Requirements

### Requirement: Tailoring starts a job-bound copy of the base CV

The system SHALL, on a tailoring bootstrap request for a vacancy, reach exactly ONE tailored CV per
(user, vacancy): it returns the caller's existing copy for that vacancy when one exists, and
otherwise creates a new CV row bound to it (`cvs.job_id` set) whose document is copied from the
user's base CV (`job_id = NULL`). It SHALL return the tailored CV id, the base CV id, and the
cached fit analysis. Both ids SHALL be the CVs' unguessable ids. The base CV MUST remain unchanged
by the bootstrap, and the tailored CV MUST be owner-scoped to the requesting user. A vacancy that
is a private job (`is_private = true`) not created by the requesting user SHALL be treated
identically to an unknown vacancy: the bootstrap is rejected and no CV is created.

Repeating the request MUST also reach the SAME conversation: the workspace is addressed by vacancy,
not by CV, so a reload re-runs this request, and minting a second conversation would rebind the CV
and orphan everything already said in the first. A bound session id that no longer resolves to a
conversation counts as none, and a fresh one is minted.

#### Scenario: Bootstrap creates a tailored copy bound to the vacancy

- **WHEN** a signed-in beta user requests tailoring for a vacancy and already has a base CV
- **THEN** a new CV is created with `job_id` set to that vacancy, its document equals the base CV's document, and the response returns both ids plus the cached analysis

#### Scenario: Repeating the bootstrap reaches the same CV and conversation

- **WHEN** the bootstrap is requested a second time for the same vacancy
- **THEN** it returns the CV and the conversation the first request produced, and no second CV, conversation or debit is created

#### Scenario: Another vacancy gets its own copy

- **WHEN** the bootstrap is requested for a different vacancy
- **THEN** a separate tailored CV is created for it

#### Scenario: The base CV is untouched by bootstrap

- **WHEN** the tailoring bootstrap creates a tailored copy
- **THEN** the base CV's document and `updated_at` are unchanged

#### Scenario: The returned ids are not guessable

- **WHEN** the bootstrap responds
- **THEN** `tailor_cv_id` and `base_cv_id` are random ids, and neither can be derived from the other or from any previously issued id

#### Scenario: The newest non-tailored CV wins

- **WHEN** a user owns several non-tailored CVs
- **THEN** the bootstrap copies the most recently edited one

#### Scenario: An orphaned tailored copy is not a candidate base

- **WHEN** the user's most recently edited vacancy-less CV is an orphaned tailored copy
- **THEN** the bootstrap copies a non-tailored CV instead

#### Scenario: A private job's owner can tailor against it

- **WHEN** the user who created a private job requests the tailoring bootstrap for its slug
- **THEN** the bootstrap proceeds exactly as it would for any other vacancy

#### Scenario: Tailoring against someone else's private job is rejected as unknown

- **WHEN** a user who did not create a private job requests the tailoring bootstrap for its slug
- **THEN** the bootstrap is rejected the same way as for an unknown vacancy, and no CV is created
