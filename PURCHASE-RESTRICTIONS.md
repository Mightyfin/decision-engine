# Manual purchase restrictions (internal, execution blocked)

Staff analysts can record the order reference, reviewed supplier party,
destination wallet and invoice/purchase-order document version for an application
awaiting review. Decision owns this restriction, not the commercial order itself.
No new order register, lender balance or payment mechanism is introduced here.

GET/POST `/v1/internal/tenants/{tenant_id}/credit/applications/{id}/purchase-restriction`
requires `credit_analyst`, a tenantless staff token, subject and environment.

POST fields:

- `order_reference`, `supplier_party_id`, `destination_wallet_id`
- `document_id`, `sha256` (already linked invoice or purchase order)
- `currency`, `maximum_amount_minor` (must match this application's request)
- `review_revision` from the review response and `reason` (10–4000 characters)

This records a staff-reviewed restriction, not credit approval, supplier KYC or
payment permission. The API now verifies exact destination ownership, currency,
tenant, legal entity and active receiving status through Wallet before recording.
The returned verification must be less than one minute old and is retained in
the restriction and offer snapshot. The normal staff decision still creates the
offer. Execution must repeat ownership and status checks inside its transaction.

The restriction is immutable. Changing it requires a new application/review;
there is no silent edit or deletion. Concurrent/stale requests return 409. After
a timeout, GET the recorded restriction and review state rather than sending a
new destination. The write and its attributable audit event commit together.

The offer captures the stored restriction, never a caller-supplied snapshot.
Existing offers are not backfilled. Neither internal restriction nor evidence
metadata is added to ordinary tenant offer JSON. Apply migration 00020 before
deploying the updated API.

Apply migration 00021 for destination verification before the new API. Configure
`DECISION_ENGINE_WALLET_BASE_URL`, `DECISION_ENGINE_WALLET_TOKEN_URL`,
`DECISION_ENGINE_WALLET_CLIENT_ID`, `DECISION_ENGINE_WALLET_CLIENT_SECRET`,
`DECISION_ENGINE_WALLET_APPLICATION_ID`, and
`DECISION_ENGINE_WALLET_LEGAL_ENTITY_ID`. Environment comes from the existing
Decision Engine environment setting. Missing configuration fails closed for new
purchase restrictions; it does not disable other credit workflows.

The dedicated service client needs Wallet roles `wallet-destination-verifier`
and `platform-tenant-delegator`, with scope `wallet.destination.verify`. These
must not be granted to tenant credentials. No role assignment or deployment has
been performed by this code change. Older restrictions without verification
remain historical records, not verified destinations; no proof is backfilled.

IMPORTANT: applications with this restriction cannot be accepted into the legacy
cash-loan handoff. Both API storage and database triggers enforce that block.
Controlled-goods execution is not enabled. No operational migration or deployment
has occurred. Do not use this unfinished workflow for live lending.
