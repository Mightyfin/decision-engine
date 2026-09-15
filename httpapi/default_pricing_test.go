package httpapi

import (
	"github.com/Mightyfin/decision-engine/pricing"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultPricingPublicationAuthority(t *testing.T) {
	body := `{"product_policy_id":"shared-credit","version":1,"currency":"ZMW","interest_method":"flat","rate_period":"monthly","interest_rate_bps":200,"penalty_basis":"overdue_principal"}`
	for _, tc := range []struct {
		name, app string
		role      bool
		want      int
	}{{"staff", "", true, 201}, {"workload", "app-test", true, 403}, {"tenant", "app-test", false, 403}} {
		t.Run(tc.name, func(t *testing.T) {
			store := &capturePricingPolicies{}
			s := Server{Auth: testAuth{Principal{Subject: "staff-test", ApplicationID: tc.app, Roles: map[string]bool{"credit_policy_admin": tc.role}}}, Policies: store}
			r := httptest.NewRequest("POST", "/v1/internal/credit/default-pricing-policies", strings.NewReader(body))
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.want == 201 && (store.tenant != pricing.DefaultOwner || store.policy.Currency != "ZMW" || store.policy.CreatedBy != "staff-test") {
				t.Fatal(store)
			}
		})
	}
}
