// Package transaction calculates wallet/payment pricing. It does not authorize
// transfers, reserve money or post a ledger. Callers must supply an authoritative
// policy snapshot selected for the authenticated tenant and product.
package transaction

import (
	"errors"
	"math/big"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid transaction pricing configuration or request")
var ErrUnavailable = errors.New("transaction pricing unavailable")

type Share struct {
	Beneficiary string `json:"beneficiary"`
	BasisPoints int64  `json:"basis_points"`
}
type Component struct {
	Code         string  `json:"code"`
	Basis        string  `json:"basis"` // transaction_value; no implicit percentage base
	Payer        string  `json:"payer"` // sender, receiver or tenant
	FixedMinor   int64   `json:"fixed_minor"`
	RateBPS      int64   `json:"rate_bps"`
	MinimumMinor int64   `json:"minimum_minor"`
	MaximumMinor *int64  `json:"maximum_minor,omitempty"` // nil = no cap; zero is explicit
	Shares       []Share `json:"shares"`
	RefundRule   string  `json:"refund_rule"` // refund_on_reversal or retain_on_reversal
}
type Policy struct {
	ID              string    `json:"id"`
	Version         int       `json:"version"`
	Currency        string    `json:"currency"`
	TransactionType string    `json:"transaction_type"`
	EffectiveFrom   time.Time `json:"effective_from"`
	EffectiveTo     time.Time `json:"effective_to"`
	QuoteTTLSeconds int64     `json:"quote_ttl_seconds"`
	// Empty components are valid only with an explicitly configured zero-fee plan.
	ZeroFee    bool        `json:"zero_fee"`
	Components []Component `json:"components"`
}
type Allocation struct {
	Beneficiary string `json:"beneficiary"`
	AmountMinor int64  `json:"amount_minor"`
}
type Line struct {
	Code        string       `json:"code"`
	Payer       string       `json:"payer"`
	Basis       string       `json:"basis"`
	AmountMinor int64        `json:"amount_minor"`
	RefundRule  string       `json:"refund_rule"`
	Allocations []Allocation `json:"allocations"`
}
type Quote struct {
	PolicyID            string    `json:"policy_id"`
	PolicyVersion       int       `json:"policy_version"`
	Currency            string    `json:"currency"`
	TransactionType     string    `json:"transaction_type"`
	TransferMinor       int64     `json:"transfer_minor"`
	SenderDebitMinor    int64     `json:"sender_debit_minor"`
	ReceiverCreditMinor int64     `json:"receiver_credit_minor"`
	TenantDebitMinor    int64     `json:"tenant_debit_minor"`
	FeeMinor            int64     `json:"fee_minor"`
	GeneratedAt         time.Time `json:"generated_at"`
	ValidUntil          time.Time `json:"valid_until"`
	Lines               []Line    `json:"lines"`
}

func Calculate(p Policy, currency, kind string, amount int64, at time.Time) (Quote, error) {
	if strings.TrimSpace(p.ID) == "" || p.Version < 1 {
		return Quote{}, ErrUnavailable
	}
	if len(p.Currency) != 3 || strings.Trim(p.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" || currency != p.Currency || kind == "" || kind != p.TransactionType || amount <= 0 || at.IsZero() || p.EffectiveFrom.IsZero() || p.QuoteTTLSeconds <= 0 || p.QuoteTTLSeconds > 86400*30 {
		return Quote{}, ErrInvalid
	}
	if !p.EffectiveTo.IsZero() && !p.EffectiveTo.After(p.EffectiveFrom) {
		return Quote{}, ErrInvalid
	}
	if at.Before(p.EffectiveFrom) || (!p.EffectiveTo.IsZero() && !at.Before(p.EffectiveTo)) {
		return Quote{}, ErrUnavailable
	}
	if (len(p.Components) == 0) != p.ZeroFee {
		return Quote{}, ErrInvalid
	}
	q := Quote{PolicyID: p.ID, PolicyVersion: p.Version, Currency: currency, TransactionType: kind, TransferMinor: amount, SenderDebitMinor: amount, ReceiverCreditMinor: amount, GeneratedAt: at.UTC(), ValidUntil: at.Add(time.Duration(p.QuoteTTLSeconds) * time.Second).UTC(), Lines: []Line{}}
	if !p.EffectiveTo.IsZero() && q.ValidUntil.After(p.EffectiveTo) {
		q.ValidUntil = p.EffectiveTo.UTC()
	}
	seen := map[string]bool{}
	for _, c := range p.Components {
		if strings.TrimSpace(c.Code) == "" || seen[c.Code] || c.Basis != "transaction_value" || c.FixedMinor < 0 || c.RateBPS < 0 || c.MinimumMinor < 0 || len(c.Shares) == 0 {
			return Quote{}, ErrInvalid
		}
		seen[c.Code] = true
		if c.RefundRule != "refund_on_reversal" && c.RefundRule != "retain_on_reversal" {
			return Quote{}, ErrInvalid
		}
		if c.MaximumMinor != nil && (*c.MaximumMinor < 0 || *c.MaximumMinor < c.MinimumMinor) {
			return Quote{}, ErrInvalid
		}
		n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(c.RateBPS))
		n.Add(n, big.NewInt(5000))
		n.Quo(n, big.NewInt(10000))
		n.Add(n, big.NewInt(c.FixedMinor))
		if n.Cmp(big.NewInt(c.MinimumMinor)) < 0 {
			n.SetInt64(c.MinimumMinor)
		}
		if c.MaximumMinor != nil && n.Cmp(big.NewInt(*c.MaximumMinor)) > 0 {
			n.SetInt64(*c.MaximumMinor)
		}
		if !n.IsInt64() {
			return Quote{}, ErrInvalid
		}
		fee := n.Int64()
		var err error
		switch c.Payer {
		case "sender":
			q.SenderDebitMinor, err = add(q.SenderDebitMinor, fee)
		case "receiver":
			if fee > q.ReceiverCreditMinor {
				return Quote{}, ErrInvalid
			}
			q.ReceiverCreditMinor -= fee
		case "tenant":
			q.TenantDebitMinor, err = add(q.TenantDebitMinor, fee)
		default:
			return Quote{}, ErrInvalid
		}
		if err != nil {
			return Quote{}, err
		}
		q.FeeMinor, err = add(q.FeeMinor, fee)
		if err != nil {
			return Quote{}, err
		}
		total := int64(0)
		beneficiaries := map[string]bool{}
		for _, s := range c.Shares {
			if strings.TrimSpace(s.Beneficiary) == "" || beneficiaries[s.Beneficiary] || s.BasisPoints <= 0 || s.BasisPoints > 10000 {
				return Quote{}, ErrInvalid
			}
			beneficiaries[s.Beneficiary] = true
			total += s.BasisPoints
			if total > 10000 {
				return Quote{}, ErrInvalid
			}
		}
		if total != 10000 {
			return Quote{}, ErrInvalid
		}
		line := Line{Code: c.Code, Payer: c.Payer, Basis: c.Basis, AmountMinor: fee, RefundRule: c.RefundRule}
		remaining := fee
		// Configured last beneficiary receives the rounding remainder. Order is part
		// of the immutable policy, so allocation is deterministic and balances exactly.
		for i, s := range c.Shares {
			v := remaining
			if i < len(c.Shares)-1 {
				part := new(big.Int).Mul(big.NewInt(fee), big.NewInt(s.BasisPoints))
				part.Quo(part, big.NewInt(10000))
				v = part.Int64()
			}
			remaining -= v
			line.Allocations = append(line.Allocations, Allocation{Beneficiary: s.Beneficiary, AmountMinor: v})
		}
		q.Lines = append(q.Lines, line)
	}
	return q, nil
}
func add(a, b int64) (int64, error) {
	n := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
