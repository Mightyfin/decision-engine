# MightyFin Decision Engine

The Decision Engine is the **Mighty Core Financial Engines** boundary. EFaaS is a distribution and orchestration layer around it; it is not another decision engine. The same product, credit/risk and pricing decisions can be consumed by direct MightyFin lending, embedded-finance products, and tenant-facing EFaaS APIs.

It is organised into three deliberately separate domains:

- **Product Engine** — eligibility, amount and term boundaries, lifecycle availability, and policy versions.
- **Credit & Risk Engine** — applications, manual review, limits/exposure, offers, acceptance and immutable decision audit.
- **Pricing Engine** — deterministic price quotes from explicit product policy inputs.

It does **not** move money, post ledger entries, create repayment schedules, calculate penalties, or bypass provider/payment controls. Payment Rails, Wallet Ledger and LMS remain the owners of those responsibilities.

The `intelligence/` area is documentation-only scaffolding for future evidence processing and model support; it contains no implemented underwriting model. The initial release is sandbox-only and manual-decision-first: every submitted credit case is reviewed by an authorised Credit Analyst. A future automated score must be a separately versioned evaluator that records its inputs, policy version, decision and reason codes.
