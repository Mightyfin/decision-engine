# Wallet transaction pricing — implementation checkpoint

## Evidence-backed gap

The current Go Wallet `internal/financial/service.go` transfer path scopes both
wallets to the same tenant/legal entity and posts the requested transfer amount.
It does not consume an itemised transaction-pricing quote. The older Adonis wallet
contains separate fee calculations; those are not evidence of integration with
the shared Go Wallet or Decision Engine.

Existing uncommitted Wallet changes were preserved. No running transfer, live
fee policy, tenant entitlement or financial posting was changed in this work.

## Implemented here

`pricing/transaction` is a pure calculator, separate from credit interest:

- Integer minor-unit arithmetic, half-up percentage rounding and checked totals.
- Fixed plus percentage fees with explicit transaction-value basis and caps/floors.
- Sender-added, receiver-deducted and tenant-paid components.
- Configured beneficiary allocations that sum exactly to the charge.
- Deterministic rounding remainder to the final configured beneficiary.
- Explicit zero-fee policy; absent configuration is never silently free.
- Currency/type matching, effective dates and bounded quote validity.
- Quote policy version, component identities and declared reversal refund rule.
- Snapshots independent of subsequent mutation of the in-memory policy.

Tests cover K1,000 plus K10 sender fee, receiver/tenant alternatives, caps/floors,
beneficiary rounding, date boundaries, invalid configuration, overflow, snapshot
stability and a conservation fuzz property.

This is NOT an authorization service, a persistent quote API, an accrual engine,
statutory revenue recognition, cross-tenant permission or a completed wallet flow.
Beneficiary IDs and policy input must come from trusted approved configuration,
not a tenant-supplied fee instruction. Provider allocations are not automatically
MightyFin revenue. The refund field declares a rule; it does not execute reversals.

## Required next integration gates

1. Governed persistent transaction policies, assignment/default resolution,
   maker/checker activation and immutable quote identity/request binding.
2. Fee quote execution in Wallet: verify tenant, source/destination, currency,
   amount, expiry, source of funds, fee payer and beneficiary account mappings.
3. Atomic sender/receiver/tenant/beneficiary postings; insufficient funds for
   principal plus fees must cause zero effects. Replay returns the original quote
   and transaction; changed requests conflict.
4. Configured refund/reversal execution once, with financial reconciliation.
5. Tenant gateway contracts plus evidence-backed sandbox deployment.
6. Same-tenant cash P2P tests, restricted goods-credit-to-cash denial, cross-tenant
   denial, and separately approved cross-network transfers where supported.

Do not weaken the existing same-tenant restriction to make a cross-network test
pass. Shared identity does not imply shared account access or transferable credit.
Do not label these local calculator tests as Green end-to-end UAT.
