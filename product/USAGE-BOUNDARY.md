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
