package transaction

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func policy() Policy {
	cap := int64(2000)
	return Policy{ID: "test-p2p", Version: 1, Currency: "ZMW", TransactionType: "p2p_transfer", EffectiveFrom: now.Add(-time.Hour), EffectiveTo: now.Add(time.Hour), QuoteTTLSeconds: 60, Components: []Component{{Code: "transfer_fee", Basis: "transaction_value", Payer: "sender", RateBPS: 100, MinimumMinor: 200, MaximumMinor: &cap, RefundRule: "refund_on_reversal", Shares: []Share{{Beneficiary: "mightyfin", BasisPoints: 10000}}}}}
}
func TestSenderReceiverTenantAndConservation(t *testing.T) {
	for _, payer := range []string{"sender", "receiver", "tenant"} {
		t.Run(payer, func(t *testing.T) {
			p := policy()
			p.Components[0].Payer = payer
			q, err := Calculate(p, "ZMW", "p2p_transfer", 100000, now)
			if err != nil {
				t.Fatal(err)
			}
			if q.FeeMinor != 1000 {
				t.Fatal(q)
			}
			if q.SenderDebitMinor+q.TenantDebitMinor != q.ReceiverCreditMinor+q.FeeMinor {
				t.Fatal("unbalanced quote", q)
			}
			if payer == "sender" && (q.SenderDebitMinor != 101000 || q.ReceiverCreditMinor != 100000) {
				t.Fatal(q)
			}
			if payer == "receiver" && (q.SenderDebitMinor != 100000 || q.ReceiverCreditMinor != 99000) {
				t.Fatal(q)
			}
			if payer == "tenant" && (q.TenantDebitMinor != 1000 || q.ReceiverCreditMinor != 100000) {
				t.Fatal(q)
			}
		})
	}
}
func TestFloorsCapsAndExplicitFree(t *testing.T) {
	for _, tc := range []struct{ amount, fee int64 }{{10000, 200}, {100000, 1000}, {500000, 2000}} {
		q, e := Calculate(policy(), "ZMW", "p2p_transfer", tc.amount, now)
		if e != nil || q.FeeMinor != tc.fee {
			t.Fatal(q, e)
		}
	}
	p := policy()
	p.Components = nil
	if _, e := Calculate(p, "ZMW", "p2p_transfer", 100, now); e == nil {
		t.Fatal("missing fee silently free")
	}
	p.ZeroFee = true
	q, e := Calculate(p, "ZMW", "p2p_transfer", 100, now)
	if e != nil || q.FeeMinor != 0 {
		t.Fatal(q, e)
	}
}
func TestSplitsAndSnapshot(t *testing.T) {
	p := policy()
	p.Components[0].RateBPS = 0
	p.Components[0].MinimumMinor = 0
	p.Components[0].FixedMinor = 101
	p.Components[0].Shares = []Share{{"mightyfin", 5000}, {"rail", 4000}, {"partner", 1000}}
	q, e := Calculate(p, "ZMW", "p2p_transfer", 100000, now)
	if e != nil {
		t.Fatal(e)
	}
	a := q.Lines[0].Allocations
	if a[0].AmountMinor != 50 || a[1].AmountMinor != 40 || a[2].AmountMinor != 11 {
		t.Fatal(a)
	}
	before, _ := json.Marshal(q)
	p.Version = 2
	p.Components[0].FixedMinor = 500
	p.Components[0].Shares[0].Beneficiary = "changed"
	after, _ := json.Marshal(q)
	if string(before) != string(after) {
		t.Fatal("policy edit mutated quote")
	}
}
func TestInvalidAndEffectiveDates(t *testing.T) {
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.ID = "" }, func(p *Policy) { p.Components[0].Basis = "" },
		func(p *Policy) { p.Components[0].Payer = "unknown" }, func(p *Policy) { p.Components[0].RefundRule = "" },
		func(p *Policy) { p.Components[0].Shares[0].BasisPoints = 9999 },
		func(p *Policy) { p.Components = append(p.Components, p.Components[0]) },
		func(p *Policy) { p.Components[0].FixedMinor = -1 }, func(p *Policy) { p.Components[0].RateBPS = -1 },
		func(p *Policy) { v := int64(100); p.Components[0].MaximumMinor = &v },
		func(p *Policy) { p.EffectiveFrom = now.Add(time.Second) }, func(p *Policy) { p.EffectiveTo = now },
	} {
		p := policy()
		mutate(&p)
		if _, e := Calculate(p, "ZMW", "p2p_transfer", 100000, now); e == nil {
			t.Fatal("invalid configuration accepted", p)
		}
	}
	p := policy()
	if _, e := Calculate(p, "USD", "p2p_transfer", 100000, now); e == nil {
		t.Fatal("currency mismatch")
	}
	if _, e := Calculate(p, "ZMW", "cash_out", 100000, now); e == nil {
		t.Fatal("wrong transaction type")
	}
	p.QuoteTTLSeconds = 7200
	q, e := Calculate(p, "ZMW", "p2p_transfer", 100000, now)
	if e != nil || !q.ValidUntil.Equal(p.EffectiveTo) {
		t.Fatal(q, e)
	}
	p = policy()
	p.Components[0].Payer = "receiver"
	if _, e := Calculate(p, "ZMW", "p2p_transfer", 1, now); e == nil {
		t.Fatal("negative receiver credit")
	}
	if _, e := Calculate(policy(), "ZMW", "p2p_transfer", math.MaxInt64, now); e == nil {
		t.Fatal("overflow")
	}
}
func FuzzConservation(f *testing.F) {
	f.Add(int64(100000), int64(100))
	f.Fuzz(func(t *testing.T, amount, rate int64) {
		if amount <= 0 || rate < 0 {
			return
		}
		p := policy()
		p.Components[0].RateBPS = rate
		q, e := Calculate(p, "ZMW", "p2p_transfer", amount, now)
		if e != nil {
			return
		}
		if q.SenderDebitMinor-q.ReceiverCreditMinor != q.FeeMinor {
			t.Fatal(q)
		}
	})
}
