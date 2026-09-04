# MightyFin Decision Engine

The Decision Engine owns versioned business-policy decisions. It is organised into three deliberately separate domains:

- **Product Engine** — eligibility, amount and term boundaries, lifecycle availability, and policy versions.
- **Credit & Risk Engine** — applications, manual review, limits/exposure, offers, acceptance and immutable decision audit.
- **Pricing Engine** — deterministic price quotes from explicit product policy inputs.

It does **not** move money, post ledger entries, create repayment schedules, calculate penalties, or bypass provider/payment controls. Payment Rails, Wallet Ledger and LMS remain the owners of those responsibilities.

The initial release is sandbox-only and manual-decision-first. A future automated score must be a separately versioned evaluator that records its inputs, policy version, decision and reason codes.
