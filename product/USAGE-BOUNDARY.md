# Product usage compatibility

The Product Engine adapter currently supports omitted/null legacy usage terms or
the explicit borrower-cash contract. It refuses restricted goods, partial cash
draws, repayment restoration and unknown usage fields. Such a product is not an
available credit policy; it must not become an ordinary cash offer by ignoring its
configuration.

This remains an admission boundary, not goods-credit execution. Supported usage
terms now travel from Product Engine into the application and offer, and the
accepted-offer event reads them from the persisted offer. Tenant-supplied usage
does not replace product configuration. The standalone pricing quote is unchanged;
usage rules are held on its associated offer, not recalculated by Pricing.

Migration 00018 must precede deployment of the new Decision API. Application and
offer usage snapshots cannot be changed, and an offer must match its application's
snapshot. Legacy records stay NULL; there is no backfill from today's product.
Explicit null legacy settings remain valid. Rolling back to old writers while
processing new snapshot-bearing applications is unsafe: the database rejects an
offer that omits its application's snapshot. Drain/redeploy Decision writers as
one release. Do not drop the migration to force a rollback.

Facility persistence, capacity reservation and restricted settlement remain open.
This event addition does not enable restricted goods, partial cash usage or
revolving restoration. Existing accepted contracts are not rewritten.

## Central policy admission checks

`Service.Validate` checks the exact tenant and product ID returned by every store,
not only the HTTP adapter. A missing or mismatched identity returns `ErrNotFound`
without returning the foreign policy. Policy ranges must be positive and ordered,
and currency must be a three-letter uppercase code. Request bounds remain
inclusive and come from the policy; the service does not invent rates or limits.

Tests cover substituted tenant/product identities, empty identities, malformed
amount/term/currency settings, and exact permitted bounds. These tests support UAT
product isolation and validation checks; they are not deployed two-tenant UAT.
These checks apply on admission, not by rewriting existing accepted contracts.
