package product

import (
	"encoding/json"
	"testing"
)

func TestCashUsageCompatibility(t *testing.T) {
	for _, raw := range []string{"", "null", `{"funding_mode":"borrower_cash","destination_rule":"borrower_wallet","allow_partial_use":false,"usage_expiry_days":0,"repayment_restoration":"none"}`} {
		if !supportsUsage(json.RawMessage(raw)) {
			t.Fatalf("cash rejected: %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"funding_mode":"restricted_goods"}`, `{"funding_mode":"borrower_cash","destination_rule":"borrower_wallet","repayment_restoration":"none","new_restriction":true}`, `{"funding_mode":"borrower_cash","destination_rule":"borrower_wallet","repayment_restoration":"principal_repaid"}`} {
		if supportsUsage(json.RawMessage(raw)) {
			t.Fatalf("unsupported usage accepted: %s", raw)
		}
	}
}
