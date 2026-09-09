# Tenant credit application drafts

Implemented locally on 2026-09-09. Not deployed or end-to-end certified.

The tenant owns its customer interface and authentication. Product Engine owns
the versioned question/document configuration; Decision Engine snapshots and
enforces it. Staff continue to decide credit manually. This does not port LMS
customer registration into eFaaS.

## Workload routes

1. `POST /v1/credit/application-drafts`: same core financial/applicant fields as
   the existing credit application POST. Party, tenant, environment and applicant
   role are required. Returns application, requirements, answers and revision.
   The base financial request must meet product limits; only questionnaire
   answers and evidence are initially incomplete.
2. `GET /v1/credit/applications/{id}/draft`: retrieve the persisted draft/snapshot.
3. `PUT /v1/credit/applications/{id}/draft`: replace all questionnaire answers
   with `{ "revision": 1, "answers": { "income": 1000 } }`. Missing required
   answers are allowed while drafting; invalid types/unknown keys are rejected.
4. Existing document access and evidence binding routes now support draft cases.
   The document service still verifies party ownership, digest and clean scan.
5. `POST /v1/credit/applications/{id}/submit`: `{ "revision": 2 }`. Rechecks the
   active product version/terms, saved answers and required bound documents via
   the document service. Success enters pending_review, not approved or funded.

The eFaaS gateway forwards these routes under credit:read/credit:write, with
its existing workload and Idempotency-Key guards. Evidence retains separate
credit:evidence:write authority. Draft writes also require a write-authorised
workload at Decision Engine, not merely credit:read. Creation derives a stable
ID from tenant, environment, application and key, stores the canonical input
hash and serialises concurrent creation using a database lock. A replay returns
the same case's current state without another creation audit; different input
under the same key returns 409. This is resource replay, not a frozen response.
New workload-created cases retain their creating application's access boundary
on Decision Engine ID routes. Human staff review retains its separate authority.

Configured requirements cannot be bypassed through the old one-step POST:
it returns draft_required. Products without any questions/documents retain
the existing flow. A draft fixes applicant and financial terms; changes need
a new draft. A newer product version returns product_version_changed rather
than silently rewriting the snapshot. Unknown/invalid fields and missing fields
and document types are returned as application_incomplete details.

Drafts stay out of the review queue. Saves and submission use database locks
and expected revisions; application changes and audit entries commit together.
No funding event, offer or ledger entry is created on draft submission. Answers
are not copied into free-text audit reasons.

While awaiting_information, authorised tenant staff can correct questionnaire
answers but cannot remove required answers. Pending-review cases stay locked.
Every saved revision retains its answers, actor and action in the append-only
credit_application_answer_history table, in the same transaction as the change.
An immutable trigger prevents update/delete of historical revisions. Initial
submission and correction use the expected revision to prevent lost updates.

## Validation and remaining release work

All Decision Engine unit tests passed. TestDraftLifecycle passed against a
disposable PostgreSQL 17 database, exercising migration replay, draft exclusion,
tenant/environment isolation, stale edits, incomplete rejection, document
verification failure, product-version changes, successful submission, duplicate
submission rejection, audit count and absence of funding events. Product and
document providers were test doubles in that integration test.

The final disposable-database run also passed information-request corrections,
retention of the original answer revision and rejection of history mutation.
The compiled tenant browser test passed answer correction and return-for-review
after an information request, in addition to the initial submission journey.

Local tests now also cover eight concurrent creation attempts yielding one
creation audit, durable retries/key conflicts, application
ownership checks and reviewer questionnaire snapshots including legacy cases.
The Admin Console has a saved applicant-response panel, with product version,
response revision and required documents; it does not label answers verified.

The Admin Console TypeScript check and targeted ESLint check passed; browser
component checks now pass for displayed answers, zero/false values, escaped text,
document disclosure and legacy/empty states. The tenant console is connected
through session BFF and gateway routes for draft answer editing/submission by
authorised tenant credit staff. Initial creation remains a workload operation.
The isolated test database was disposable, with
no operational data or provider calls.

Both frontend production builds and compiled-app browser tests have now passed
using signed synthetic sessions and isolated API fixtures. The tenant test
covers the draft/evidence/submission and information-request recovery flow
through the actual session BFF. The admin test covers the review page and
non-analyst denial. This is stronger than component rendering but does not
certify production identity/provider connectivity.

Before deployment: certify application-level
credential isolation end-to-end with real tokens, live party/relationship
authorisation, and authenticated deployed browser journeys; exercise
real document/token integration and browser flows, and validate bounded request
timeouts under larger evidence sets. Existing responses with no draft metadata
must remain readable. The migration must precede application deployment.

This is not a claim that the complete tenant onboarding-to-funding journey is
finished. No production migrations or customer-data changes were performed.
