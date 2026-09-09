package creditrisk

import (
	"encoding/json"
	"github.com/Mightyfin/decision-engine/product"
)

// Questionnaire is the saved applicant response, not a verified assessment.
type Questionnaire struct {
	ProductVersion        int                        `json:"product_version"`
	Revision              int                        `json:"revision"`
	Fields                []product.Field            `json:"fields"`
	Answers               map[string]json.RawMessage `json:"answers"`
	RequiredDocumentTypes []string                   `json:"required_document_types"`
}
