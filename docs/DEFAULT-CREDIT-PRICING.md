# Shared default credit pricing

Implemented in Decision Engine; requires migration 00024 and a tested service
deployment. No Green configuration or production rates are seeded by this change.

## Selection

1. An active policy for the exact tenant and product wins (complete override).
2. Otherwise use the active MightyFin default explicitly bound by staff to that
   tenant/product, matching the configured currency.
3. No binding/default means pricing is unavailable, never a zero-price fallback.

Default keys are administrator-defined pricing plans, not inferred from tenant
names, product names or distribution channels. Different products across direct
lending, EF and eFaaS may share a key when their commercial terms are meant to
match. Merely sharing a name does not assign pricing. Consumers must use the
Decision Engine to benefit; this change does not rewrite the Laravel LMS.

## Staff configuration API

All writes require `credit_policy_admin`; workload application identities are
denied even if that role was mistakenly assigned. Use authenticated staff sessions.

- `POST /v1/internal/credit/default-pricing-policies`: same body as existing
  tenant policy publishing, with required `currency`. `product_policy_id` is the
  shared default key here. Example **synthetic test only**: flat, monthly,
  `interest_rate_bps: 200` (2%), version 1, currency ZMW.
- `PUT /v1/internal/tenants/{tenant_id}/credit/products/{product_id}/default-pricing`:
  `{"default_policy_key":"approved-shared-key","currency":"ZMW"}`.
- Existing tenant pricing publication remains the full-override path. Rates must
  be supplied explicitly, including intended zeros; this is not a partial patch.

Publishing requires an increasing version and serializes concurrent writes.
The preceding version becomes inactive, not deleted. Configuration writes and
their actor are recorded transactionally in an append-only audit table. New quote
charge lines capture source, policy key and version alongside calculated amounts;
the existing offer snapshot stores those lines. Later configuration does not
recalculate previously saved offers.

## Boundaries still open

- Admin portal editing/publishing forms and gateway integration are not added.
- Channel overrides, component-by-component inheritance, future effective dates,
  expiry, separate maker/checker publication and override retirement are not
  implemented by this change. Publishing is immediate under the existing
  policy-administrator authority, not a new approval workflow.
- Platform subscriptions/usage pricing, split payers, tax and statutory revenue
  recognition remain separate capabilities and are not certified here.
- Deployment, Green product assignment, staff credit/funding approval and full
  Green financial UAT remain outstanding.

## Verification

`go test ./...` and targeted `go vet` check package regressions. For real PostgreSQL
migration and resolver tests, provide `DECISION_DEFAULT_PRICING_TEST_DATABASE_URL`
pointing to a disposable database, then run
`go test -race ./storage -run TestDefaultPricingResolution -count=1`.
The 15 September run passed this test without `-race`, including concurrent
publication, plus the full package suite and targeted vet. The race detector
could not run because the host has no C compiler; it is not claimed as passed.
The test creates and removes its own schema. It checks default fallback,
tenant-specific precedence and isolation, currency mismatch, missing configuration,
version changes, captured old quote terms, concurrent publication, policy-ID
uniqueness across tenants, and audit mutation denial. No live-provider or accounting
compliance certification is implied.
