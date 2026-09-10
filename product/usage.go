package product

import (
	"bytes"
	"encoding/json"
	"io"
)

// Until approved usage snapshots and capacity execution are implemented, this
// adapter may only map the historical cash contract. Never silently drop a new
// product's financial restrictions while constructing an executable policy.
func supportsUsage(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	var u struct {
		FundingMode          string `json:"funding_mode"`
		DestinationRule      string `json:"destination_rule"`
		AllowPartialUse      bool   `json:"allow_partial_use"`
		UsageExpiryDays      int    `json:"usage_expiry_days"`
		RepaymentRestoration string `json:"repayment_restoration"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(&u) == nil && d.Decode(&struct{}{}) == io.EOF && u.FundingMode == "borrower_cash" && u.DestinationRule == "borrower_wallet" && !u.AllowPartialUse && u.UsageExpiryDays == 0 && u.RepaymentRestoration == "none"
}
