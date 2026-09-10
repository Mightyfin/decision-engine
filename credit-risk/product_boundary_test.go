package creditrisk

import (
	"context"
	"errors"
	"testing"

	"github.com/Mightyfin/decision-engine/product"
)

func TestForeignProductCannotCreateApplicationOrOffer(t *testing.T) {
	for _, foreign := range []product.Policy{
		{ID: "p", TenantID: "other-tenant"},
		{ID: "other-product", TenantID: "t"},
	} {
		foreign.Active = true
		foreign.Version = 1
		foreign.Currency = "ZMW"
		foreign.MinimumAmount, foreign.MaximumAmount = 100, 10000
		foreign.MinimumTermDays, foreign.MaximumTermDays = 1, 90
		foreign.RepaymentIntervalDays = 30
		foreign.AllocationOrder = []string{"penalty", "fees", "interest", "principal"}
		store := &memory{a: map[string]Application{}, o: map[string]Offer{}}
		service := Service{Store: store, Products: product.Service{Store: products{foreign}}}
		_, err := service.Submit(context.Background(), Application{ID: "app", TenantID: "t", ProductPolicyID: "p", RelationshipID: "rel", Currency: "ZMW", Purpose: "stock", Amount: 1000, TermDays: 30}, "tenant-user")
		if !errors.Is(err, product.ErrNotFound) {
			t.Fatalf("expected policy ownership rejection: %v", err)
		}
		if len(store.a) != 0 || len(store.o) != 0 || len(store.audit) != 0 {
			t.Fatal("rejected product created application, offer or success audit")
		}
	}
}
