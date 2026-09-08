# Credit evidence binding — staged implementation

POST /v1/credit/applications/{id}/evidence accepts document_id and sha256 only.
Requires credit:evidence:write (credit_evidence_writer), authenticated tenant,
environment and subject, plus a delegated token accepted by the document API
with documents.evidence.verify. No automatic permission grants are included.
DECISION_ENGINE_DOCUMENT_BASE_URL enables the adapter; absent configuration
returns unavailable. Do not use a tenant-unscoped service credential.

The case supplies the subject. The document service verifies the exact version,
tenant, environment, subject and clean scan. A transaction rechecks the case is
pending_review and stores an append-only binding with attributable audit. Repeating
the same binding is idempotent. This is evidence registration, not a decision or
document content assessment. A future decision workflow must reverify current
versions and guard against evidence changing while the analyst reviews it.

Migration 00009 must precede deployment. New applications record the environment
from the verified token in their creation transaction. Existing applications have
no inferred environment and cannot receive evidence until their provenance is
validated through an audited migration. No production backfill is supplied.

Not yet delivered: tenant gateway forwarding and delegated document audiences,
evidence listing/download UI, uploads, requirements, resubmission and stale-review
protection. Do not expose the binding endpoint to staff/tenants until integration
permissions and these workflows are verified. Existing decision behaviour is not
changed by this staged addition. No live migration or deployment in this change.
