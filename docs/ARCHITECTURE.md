# Decision boundaries

```text
EFaaS API → Decision Engine → Product policy / Price quote / Credit decision
                                  ↓
                         accepted offer event
                                  ↓
                    Payment Rails + Wallet Ledger (separate services)
                                  ↓
                          LMS loan lifecycle (separate system)
```

An offer is a decision, not a disbursement instruction. Acceptance does not reserve funds or create a loan. Downstream services consume a future `credit.offer.accepted` event only after their own controls approve action.

All amounts are minor units in domain code. Database migrations use `numeric(18,2)` at the persistence boundary. Product and pricing policy carry versions, preventing changed settings from silently rewriting historic applications or offers.
