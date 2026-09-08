package creditrisk

// AssessmentContext describes the recorded subject, not a verification result.
// A tenant boundary identifies the submitting network; it is not the borrower.
type AssessmentContext struct {
	CaseType           string `json:"case_type"`
	Scenario           string `json:"scenario"`
	SubjectPartyID     string `json:"subject_party_id"`
	SubmittingTenantID string `json:"submitting_tenant_id"`
	RelationshipID     string `json:"relationship_id"`
	Channel            string `json:"channel"`
	SubjectStatus      string `json:"subject_status"`
}

func (a Application) AssessmentContext() AssessmentContext {
	c := AssessmentContext{CaseType: "credit_application", Scenario: "unclassified", SubjectPartyID: a.PartyID, SubmittingTenantID: a.TenantID, RelationshipID: a.RelationshipID, Channel: a.Origin, SubjectStatus: "incomplete"}
	switch a.ApplicantRole {
	case "network_participant":
		c.Scenario = "participant_financing"
	case "partner_organisation":
		c.Scenario = "organisation_financing"
	}
	if a.PartyID != "" && a.RelationshipID != "" && c.Scenario != "unclassified" {
		c.SubjectStatus = "recorded_not_verified"
	}
	return c
}
