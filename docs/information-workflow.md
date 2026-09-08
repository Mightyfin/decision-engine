# Information requests and review concurrency

Staged, not deployed. Apply migration 00010 before the new backend. Update the
staff console alongside it: manual decision requests now require review_revision.
Do not deploy this API alone behind an older decision form.

- Staff POST /v1/internal/tenants/{tenant}/credit/applications/{id}/information-request
  requires credit_analyst, known matching environment, reason and review_revision.
- Tenant GET /v1/credit/applications/{id}/information-request returns only the
  latest information request, current status and review_revision, not internal
  decision/audit rationale.
- Tenant POST /v1/credit/applications/{id}/resubmit requires credit:evidence:write,
  its own tenant/environment, reason and review_revision. The gateway forwards it.

pending_review -> awaiting_information -> pending_review. Each transition is
atomic with attributable audit. Evidence can be linked while awaiting information.
Resubmission does not approve a case or assert all evidence requirements are met.
No automated notifications are sent by these endpoints.

Review version is the latest audit event ID, captured before case content is read.
Information transitions and manual decisions lock the application row, then compare
the expected version. Evidence writes use the same lock and append audit, making
previous confirmations stale. Request information and response text should never
contain confidential internal fraud rules because the request is tenant-visible.

Unknown historical environments are not inferred. Their information workflow is
blocked until provenance is established. This is not a migration/backfill policy.
Production activation still requires tenant credential scope reconciliation and
the document upload/viewing workflow. No requirement engine or automatic approval
is introduced here.
