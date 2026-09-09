package product

import (
	"encoding/json"
	"strings"
	"time"
)

// Wire contract owned/configured by Product Engine. Decision Engine interprets
// this snapshot; it does not invent product-specific questions or documents.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}
type Requirements struct {
	Fields                []Field  `json:"fields"`
	RequiredDocumentTypes []string `json:"required_document_types"`
}
type Completion struct {
	Complete         bool     `json:"complete"`
	MissingFields    []string `json:"missing_fields"`
	InvalidFields    []string `json:"invalid_fields"`
	MissingDocuments []string `json:"missing_documents"`
}

func (p Policy) RequirementsFor(role string) Requirements {
	r := p.ApplicationRequirements[role]
	r.Fields = append([]Field{}, r.Fields...)
	r.RequiredDocumentTypes = append([]string{}, r.RequiredDocumentTypes...)
	for _, kind := range p.RequiredDocumentTypes {
		if !has(r.RequiredDocumentTypes, kind) {
			r.RequiredDocumentTypes = append(r.RequiredDocumentTypes, kind)
		}
	}
	return r
}
func has(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// documents must come from server-verified evidence, never request JSON.
func (r Requirements) Check(answers map[string]json.RawMessage, documents []string) Completion {
	c := Completion{Complete: true, MissingFields: []string{}, InvalidFields: []string{}, MissingDocuments: []string{}}
	known := map[string]bool{}
	for _, f := range r.Fields {
		known[f.Key] = true
		raw, exists := answers[f.Key]
		missing := !exists || strings.TrimSpace(string(raw)) == "null" || len(raw) == 0
		var text string
		if json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) == "" {
			missing = true
		}
		if missing {
			if f.Required {
				c.MissingFields = append(c.MissingFields, f.Key)
			}
			continue
		}
		valid := false
		switch f.Type {
		case "text":
			valid = json.Unmarshal(raw, &text) == nil && len(text) <= 10000
		case "date":
			if json.Unmarshal(raw, &text) == nil {
				_, err := time.Parse("2006-01-02", text)
				valid = err == nil
			}
		case "select":
			valid = json.Unmarshal(raw, &text) == nil && has(f.Options, text)
		case "boolean":
			var v bool
			valid = json.Unmarshal(raw, &v) == nil
		case "number":
			var v float64
			valid = json.Unmarshal(raw, &v) == nil
		}
		if !valid {
			c.InvalidFields = append(c.InvalidFields, f.Key)
		}
	}
	for key := range answers {
		if !known[key] {
			c.InvalidFields = append(c.InvalidFields, key)
		}
	}
	for _, kind := range r.RequiredDocumentTypes {
		if !has(documents, kind) {
			c.MissingDocuments = append(c.MissingDocuments, kind)
		}
	}
	c.Complete = len(c.MissingFields)+len(c.InvalidFields)+len(c.MissingDocuments) == 0
	return c
}
