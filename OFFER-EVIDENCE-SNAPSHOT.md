# Evidence retained with staff decisions

New offers capture the application's linked document versions in the same
transaction that creates the offer. The database derives the snapshot from
tenant-, environment- and party-matched evidence records. Supplied snapshots
are ignored. The application lock serializes evidence linking and staff decisions.

The snapshot cannot be changed after creation. Existing offers retain NULL;
we do not reconstruct historical evidence from today's records. For new offers,
an empty array means no linked evidence was captured, not that evidence was lost.

The staff review API exposes `offer_evidence_snapshot` separately from the offer.
It is omitted from ordinary Offer JSON to avoid adding internal evidence metadata
to tenant responses or existing public events. Each entry records the document
ID, SHA-256 version, party, tenant, environment, type, linking actor and timestamp.
These are metadata, not downloadable documents or freshly verified scan results.

The review response's `document_evidence_status` describes current linked evidence:
`linked`, `not_linked`, `application_environment_unknown`, or
`integration_unavailable` when the adapter does not provide evidence reads.
Database failures return 503 instead of pretending that no evidence exists.

## Limits and rollout

Apply migration 00019 before deploying the updated Decision API. No historical
backfill is needed. Restricted-goods execution remains blocked. A linked invoice
is not proof that staff approved its supplier or payment destination. Immutable
supplier/order approval, downstream consumption and settlement still require
implementation. This increment changes neither tenant approval authority nor
MightyFin's manual credit/funding authority.
