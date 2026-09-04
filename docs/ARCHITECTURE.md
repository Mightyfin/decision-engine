# Mighty Core Financial Engines

```text
                         MIGHTY CORE FINANCIAL ENGINES
                                      |
           +--------------------------+--------------------------+
           |                          |                          |
     Product Engine            Credit & Risk Engine         Pricing Engine
     What can be offered?      Can the customer use it?      What does it cost?
           +--------------------------+--------------------------+
                                      |
                              Decision / Quote
                                      |
          +---------------------------+---------------------------+
          |                           |                           |
   Direct Lending                 Embedded Finance               EFaaS
   MightyFin LMS                  Mighty ecosystem               tenant/API distribution
```

An offer is a decision, not a disbursement instruction. Acceptance does not reserve funds or create a loan. Downstream services consume a future `credit.offer.accepted` event only after their own controls approve action. For example, an EFaaS tenant such as Hrvst can submit a credit evaluation request, but EFaaS must call this engine and return its tenant-scoped outcome rather than reproduce underwriting logic.

All amounts are minor units in domain code. Database migrations use `numeric(18,2)` at the persistence boundary. Product and pricing policy carry versions, preventing changed settings from silently rewriting historic applications or offers.

## Intelligence boundary

`intelligence/documents`, `extraction`, `anomaly-detection`, `recommendations`, `feature-generation`, and `model-scoring` may prepare evidence or recommendations. They cannot approve, decline, price, disburse, or mutate an application directly. Only a versioned policy/decision command with an immutable audit record can change the credit lifecycle.
