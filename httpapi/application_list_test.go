package httpapi

import (
	"context"
	"encoding/json"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"net/http/httptest"
	"testing"
)

type tenantListStore struct {
	testStore
	scope  creditrisk.ApplicationListScope
	filter creditrisk.ApplicationListFilter
	calls  int
}

func (s *tenantListStore) ListApplications(_ context.Context, scope creditrisk.ApplicationListScope, f creditrisk.ApplicationListFilter) ([]creditrisk.Application, error) {
	s.scope = scope
	s.filter = f
	s.calls++
	if f.Cursor == "foreign" {
		return nil, creditrisk.ErrInvalidCursor
	}
	return []creditrisk.Application{{ID: "cap_a"}, {ID: "cap_b"}}, nil
}
func TestTenantApplicationList(t *testing.T) {
	for _, tc := range []struct {
		query  string
		role   bool
		tenant string
		want   int
	}{
		{"?limit=1&status=offered&relationship_id=par_a", true, "tenant_a", 200},
		{"?limit=101", true, "tenant_a", 400}, {"?limit=0", true, "tenant_a", 400},
		{"?limit=x", true, "tenant_a", 400}, {"?limit=1&limit=2", true, "tenant_a", 400},
		{"?tenant_id=other", true, "tenant_a", 400}, {"?cursor=foreign", true, "tenant_a", 400},
		{"", false, "tenant_a", 403}, {"", true, "", 403},
	} {
		t.Run(tc.query+tc.tenant, func(t *testing.T) {
			s := &tenantListStore{}
			p := Principal{TenantID: tc.tenant, ApplicationID: "app_a", Environment: "sandbox", Roles: map[string]bool{"decision_workload": tc.role}}
			w := httptest.NewRecorder()
			Server{Auth: testAuth{p}, Applications: s}.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/credit/applications"+tc.query, nil))
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.want == 200 {
				if s.scope.TenantID != "tenant_a" || s.scope.Environment != "sandbox" || s.scope.CallerApplicationID != "app_a" || s.filter.Limit != 2 || s.filter.Status != "offered" || s.filter.RelationshipID != "par_a" {
					t.Fatal(s)
				}
				var body struct {
					Data []creditrisk.Application `json:"data"`
					Page struct {
						HasMore bool   `json:"has_more"`
						Next    string `json:"next_cursor"`
					} `json:"page"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if len(body.Data) != 1 || body.Data[0].ID != "cap_a" || !body.Page.HasMore || body.Page.Next != "cap_a" {
					t.Fatal(body)
				}
			} else if tc.query != "?cursor=foreign" && s.calls != 0 {
				t.Fatal("invalid request reached storage")
			}
		})
	}
}
