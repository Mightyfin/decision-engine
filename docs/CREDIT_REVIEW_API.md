# Manual Credit Review API — first release

The Decision Engine exposes two authenticated interfaces. EFaaS is the tenant-facing adapter; it does not decide credit.

| Caller | Operation | Result |
|---|---|---|
| EFaaS workload | `POST /v1/credit/applications` | creates `pending_review` |
| EFaaS workload | `GET /v1/credit/applications/{id}` | tenant-scoped case status / offer outcome |
| EFaaS workload | `POST /v1/credit/applications/{id}/accept` | records acceptance of an unexpired offer |
| Credit Analyst | `GET /v1/internal/tenants/{tenant_id}/credit/review-queue` | analyst work queue |
| Credit Analyst | `POST /v1/internal/credit/applications/{id}/decision` | records `offer` or `decline` with mandatory reason |

The decision endpoint loads the active product and pricing policy server-side. It requires a Credit Analyst role and records an immutable audit row. It never disburses, reserves cash, posts a ledger entry, or creates an LMS loan.

`POST /v1/credit/applications` also accepts optional applicant-reference fields:
`party_id`, `applicant_role` (`network_participant` or `partner_organisation`),
`wallet_id`, and `origin` (`direct_lending`, `embedded_finance`, or `efaas`).
They preserve the shared identity context for one credit case; verification through
the Party and Wallet service contracts is introduced separately and must not be
inferred merely because a reference was supplied.

Acceptance is the final Decision Engine lifecycle step. A downstream lending or wallet system must perform its own separate funding, disbursement, accounting and loan-lifecycle controls before any money moves.
