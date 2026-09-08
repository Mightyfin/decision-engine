package creditrisk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type evidenceFixture struct {
	a              Application
	writes, checks int
	failure        error
}

func (f *evidenceFixture) EvidenceApplication(context.Context, string) (Application, error) {
	return f.a, nil
}
func (f *evidenceFixture) BindEvidence(context.Context, Evidence) error { f.writes++; return nil }
func (f *evidenceFixture) VerifyEvidence(context.Context, Application, string, string) (string, error) {
	f.checks++
	return "BANK_STATEMENT", f.failure
}
func TestEvidenceBindingScope(t *testing.T) {
	for _, name := range []string{"valid", "foreign tenant", "foreign environment", "missing party", "legacy environment", "decided", "scan unavailable"} {
		t.Run(name, func(t *testing.T) {
			f := &evidenceFixture{a: Application{ID: "a", TenantID: "t", Environment: "sandbox", PartyID: "p", RelationshipID: "r", ApplicantRole: "network_participant", Status: "pending_review"}}
			switch name {
			case "foreign tenant":
				f.a.TenantID = "other"
			case "foreign environment":
				f.a.Environment = "production"
			case "missing party":
				f.a.PartyID = ""
			case "legacy environment":
				f.a.Environment = ""
			case "decided":
				f.a.Status = "offered"
			case "scan unavailable":
				f.failure = errors.New("unavailable")
			}
			err := (EvidenceService{Store: f, Verifier: f}).Bind(context.Background(), "a", "t", "sandbox", "actor", "doc", strings.Repeat("a", 64))
			if name == "valid" {
				if err != nil || f.writes != 1 {
					t.Fatal(err)
				}
			} else {
				if err == nil || f.writes != 0 {
					t.Fatal("unsafe write", err)
				}
				if name != "scan unavailable" && f.checks != 0 {
					t.Fatal("unauthorised document lookup")
				}
			}
		})
	}
}
