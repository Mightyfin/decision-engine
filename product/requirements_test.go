package product

import (
	"encoding/json"
	"testing"
)

func TestCompletionDoesNotTreatFalseOrZeroAsMissing(t *testing.T) {
	r := Requirements{Fields: []Field{{Key: "has_loans", Type: "boolean", Required: true}, {Key: "income", Type: "number", Required: true}, {Key: "date", Type: "date", Required: true}}, RequiredDocumentTypes: []string{"IDENTITY"}}
	a := map[string]json.RawMessage{"has_loans": json.RawMessage(`false`), "income": json.RawMessage(`0`), "date": json.RawMessage(`"2026-09-09"`)}
	if c := r.Check(a, []string{"IDENTITY"}); !c.Complete {
		t.Fatal(c)
	}
	a["date"] = json.RawMessage(`"2026-02-30"`)
	if c := r.Check(a, nil); c.Complete || len(c.InvalidFields) != 1 || len(c.MissingDocuments) != 1 {
		t.Fatal(c)
	}
	a["income"] = json.RawMessage(`null`)
	a["extra"] = json.RawMessage(`true`)
	if c := r.Check(a, nil); len(c.MissingFields) != 1 || len(c.InvalidFields) != 2 {
		t.Fatal(c)
	}
}
