# Manual Credit Review API — first release

The Decision Engine exposes two authenticated interfaces. EFaaS is the tenant-facing adapter; it does not decide credit.

| Caller | Operation | Result |
|---|---|---|
| EFaaS workload | `POST /v1/credit/applications` | creates `pending_review` |
| EFaaS workload | `GET /v1/credit/applications/{id}` | tenant-scoped case status / offer outcome |
| Credit Analyst | `GET /v1/internal/tenants/{tenant_id}/credit/review-queue` | analyst work queue |
| Credit Analyst | `POST /v1/internal/credit/applications/{id}/decision` | records `offer` or `decline` with mandatory reason |

The decision endpoint loads the active product and pricing policy server-side. It requires a Credit Analyst role and records an immutable audit row. It never disburses, reserves cash, posts a ledger entry, or creates an LMS loan.
