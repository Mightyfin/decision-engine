# MightyFin Decision Engine

The Decision Engine is the **Mighty Core Financial Engines decision boundary**. EFaaS is a distribution and orchestration layer around it; it is not another decision engine. The same credit/risk and pricing decisions can be consumed by direct MightyFin lending, embedded-finance products, and tenant-facing EFaaS APIs.

It is organised into three deliberately separate domains:

- **Product Engine integration** — the external `product-engine` owns product definitions, eligibility, amount and term boundaries, lifecycle availability, and product policy versions. This service consumes immutable product-policy snapshots; it does not maintain a second product catalogue.
- **Credit & Risk Engine** — applications, manual review, limits/exposure, offers, acceptance and immutable decision audit.
- **Pricing domain** — deterministic quotes from explicit, versioned product-pricing inputs. Pricing remains a domain inside this repository; there is no standalone Pricing Engine service or repository.

This repository is the canonical runtime owner of Credit & Risk and quote calculation. The historical standalone `credit-risk-engine` prototype is superseded and must not be deployed. Billing & Collections consumes the resulting immutable terms to invoice, schedule and collect; it does not originate or silently recalculate a product's price.

It does **not** move money, post ledger entries, create repayment schedules, calculate penalties, or bypass provider/payment controls. Payment Rails, Wallet Ledger and LMS remain the owners of those responsibilities.

The `intelligence/` area is documentation-only scaffolding for future evidence processing and model support; it contains no implemented underwriting model. The initial release is sandbox-only and manual-decision-first: every submitted credit case is reviewed by an authorised Credit Analyst. A future automated score must be a separately versioned evaluator that records its inputs, policy version, decision and reason codes.
