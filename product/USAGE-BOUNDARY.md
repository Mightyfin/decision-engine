# Product usage compatibility

The Product Engine adapter currently supports omitted/null legacy usage terms or
the explicit borrower-cash contract. It refuses restricted goods, partial cash
draws, repayment restoration and unknown usage fields. Such a product is not an
available credit policy; it must not become an ordinary cash offer by ignoring its
configuration.

This is an admission boundary, not goods-credit decisioning. The next increment
must carry the supported immutable usage terms through assessment, quote, offer
acceptance and the Facility event before restricted goods can be enabled. Existing
accepted contracts must not be recalculated from a later product version.

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
