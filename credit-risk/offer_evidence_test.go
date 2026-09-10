package creditrisk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOfferDoesNotExposeInternalEvidenceToTenantJSON(t *testing.T) {
	raw, err := json.Marshal(Offer{PurchaseRestriction: &PurchaseRestriction{DestinationWalletID: "private-destination"}, EvidenceSnapshot: []Evidence{{DocumentID: "private-review-document", LinkedBy: "internal-reviewer"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-destination") || strings.Contains(string(raw), "private-review-document") || strings.Contains(string(raw), "internal-reviewer") || strings.Contains(string(raw), "evidence_snapshot") {
		t.Fatal("internal evidence exposed", string(raw))
	}
}
