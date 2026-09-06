# Canonical applicant identity and wallet boundary

## Rule

Every credit assessment, whether originated by MightyFin Direct Lending, an
embedded-finance partner, or an EFaaS API client, is made against one canonical
`party_id` from the Party Platform. A partner organisation and a network
participant are different applicant roles, not different identity systems.

The wallet ledger must use that same `party_id` as its `owner_id`. This gives
the platforms a shared, auditable owner reference without allowing the Decision
Engine to read either platform's database directly.

`relationship_id` is not sufficient proof of identity. It remains an external
relationship reference only.

## Wallet clarification

"One wallet" means one authoritative wallet relationship and ledger owner, not
one physical wallet record forever. A party can have separate wallets for a
currency, legal entity, or permitted product. The Wallet Ledger enforces the
owner/currency/legal-entity uniqueness boundary; it remains the only service
that owns balances and wallet lifecycle.

## Required assessment reference

All new credit applications must eventually carry:

| Field | Meaning |
|---|---|
| `party_id` | Canonical Party Platform ID of the applicant |
| `applicant_role` | `network_participant` or `partner_organisation` |
| `relationship_id` | Origin-system relationship reference, for traceability |
| `wallet_id` | Authoritative wallet selected for the facility, if funds can be paid into a wallet |
| `origin` | `direct_lending`, `embedded_finance`, or `efaas` — distribution context only |

Product eligibility, pricing and analyst decisions are intentionally independent
of `origin`. This prevents direct lending and EFaaS from becoming separate
credit engines.

## Delivery order

1. Add a read-only Party Platform resolution lookup and a Wallet Ledger
   owner lookup; each returns only records within the caller's tenant and
   environment boundary.
2. Add a Decision Engine subject-verifier port that calls those public service
   contracts, never their databases.
3. Persist the verified subject reference and applicant role with a credit case.
4. Require the verified party reference for new applications; require a wallet
   reference before any downstream disbursement, not merely before assessment.
5. Aggregate exposure and future risk evidence by canonical `party_id`, while
   preserving partner/tenant isolation and analyst audit records.

Until steps 1–3 are live, an application can be manually reviewed, but it must
not be described as a cross-network identity- or wallet-verified assessment.
