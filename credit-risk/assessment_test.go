package creditrisk

import "testing"

func TestAssessmentDoesNotConfuseTenantWithSubject(t *testing.T) {
	for role, scenario := range map[string]string{"network_participant": "participant_financing", "partner_organisation": "organisation_financing", "": "unclassified"} {
		a := Application{TenantID: "tenant", PartyID: "borrower", RelationshipID: "relationship", ApplicantRole: role, Origin: "efaas"}
		c := a.AssessmentContext()
		if c.SubjectPartyID != "borrower" || c.SubmittingTenantID != "tenant" || c.Scenario != scenario || c.CaseType != "credit_application" {
			t.Fatalf("incorrect context: %+v", c)
		}
		a.PartyID = ""
		if c = a.AssessmentContext(); c.SubjectPartyID != "" || c.SubjectStatus != "incomplete" {
			t.Fatalf("invented identity: %+v", c)
		}
	}
}
