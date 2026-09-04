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

## Operating model

EFaaS submits an evaluation request to the Decision Engine. A versioned scoring adapter produces an explainable score and reason codes; the active tenant credit-routing policy returns `offered`, `declined`, or `referred`. Credit analysts govern product, pricing and routing-policy versions; they work only the referred queue, policy changes, model monitoring and approved overrides. They do not manually process normal EFaaS decisions.
