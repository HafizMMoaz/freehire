## 1. Schema: de-authored community content

- [x] 1.1 Add `migrations/0041_threads_author_set_null.sql`: drop `NOT NULL` on `threads.author_user_id` and recreate `threads_author_user_id_fkey` as `ON DELETE SET NULL`
- [x] 1.2 Change every `community_personas` join in `internal/db/queries/community.sql` from `JOIN` to `LEFT JOIN` (thread read, both list queries, reply reads) so a de-authored thread still appears; regenerate with `make sqlc`
- [x] 1.3 Render a de-authored author as a marker distinct from `aiAuthor` in `internal/handler/community.go` (thread and reply responses), covered by a handler test asserting a deleted author is not labelled "AI"

## 2. Session termination

- [ ] 2.1 Add a `UserExists(ctx, id) (bool, error)` query to `internal/db/queries/users.sql` and regenerate
- [ ] 2.2 Extend `auth.RequireAuth` and `auth.RequireAuthOrKey` with a user-existence check behind a narrow interface, returning `401` when the subject is gone; tests cover valid-token-missing-user for both the cookie and the API-key path
- [ ] 2.3 Wire the checker at every `RequireAuth`/`RequireAuthOrKey` construction site in `internal/handler/handler.go`
- [ ] 2.4 Update `internal/auth/AGENTS.md` with the existence check and why a valid signature is no longer sufficient

## 3. Erasure orchestration (`internal/accountdelete`)

- [x] 3.1 Add the SQL the service needs to `internal/db/queries/users.sql`: `DeleteUser`, `ListUserEmailObjectKeys` (non-null `emails.s3_key`), `ListUserReferralProofKeys` (`referral_offers.proof_object_key`); regenerate
- [x] 3.2 Create `internal/accountdelete` with `Service.Delete(ctx, userID)` and its narrow dependency interfaces (repository, blob store, Gmail revoker); test-drive the ordering: keys collected → revoke → objects deleted → rows deleted
- [x] 3.3 Test: a blob-store failure aborts before any row is deleted and surfaces a retryable error
- [x] 3.4 Test: a revoke failure is logged and does not stop the deletion
- [x] 3.5 Test: nil blob store (storage unconfigured) and nil revoker (Gmail unconfigured) both delete successfully

## 4. Endpoint

- [x] 4.1 Add `internal/handler/me_delete.go`: `DELETE /api/v1/me`, cookie-only, case-insensitive confirmation of the caller's own email, `204` plus an expired session cookie on success, `400` on mismatch
- [x] 4.2 Register the route under `auth.RequireAuth` (never `keyAuth`) in `internal/handler/handler.go`; test that a Bearer API key gets `401`
- [x] 4.3 Integration test (`-tags=integration`): seed a user with job tracking, a CV, credits, saved searches, mail, a thread with another member's reply, and a referral offer; delete; assert no user-owned row survives, the other member's reply survives de-authored, the moderator trail is nulled, and the objects are gone from the fake store
- [x] 4.4 Integration test: the deleted account's email can register a fresh, empty account
- [x] 4.5 Document the endpoint in `internal/handler/AGENTS.md`

## 5. Web surface

- [ ] 5.1 Add the delete-account danger zone to `web/src/routes/my/profile`: states that deletion is permanent and unrecoverable, lists what is erased (CV, mail, analyses, credits, community handle), and notes what survives de-authored
- [ ] 5.2 Gate the action on the typed email matching the signed-in address; keep it disabled until it does
- [ ] 5.3 On success clear client session state and redirect to the public site; verify visually per `web/AGENTS.md`

## 6. Ship

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...`, plus the integration tag suite for the new tests
- [ ] 6.2 Note in the change that migration 0041 must be applied by hand on prod BEFORE the binary deploys
- [ ] 6.3 Offer a changelog entry (`write-changelog`) — this is user-facing
