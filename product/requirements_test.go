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

func TestConfiguredAffirmativeAttestation(t *testing.T) {
	r := Requirements{Fields: []Field{{Key: "authority", Type: "boolean", Required: true, RequireTrue: true}}}
	for _, raw := range []string{"null", "false", `"true"`, "1", "{}"} {
		if c := r.Check(map[string]json.RawMessage{"authority": json.RawMessage(raw)}, nil); c.Complete {
			t.Fatalf("non-affirmative attestation accepted: %s", raw)
		}
	}
	if r.Check(nil, nil).Complete {
		t.Fatal("missing attestation accepted")
	}
	if !r.Check(map[string]json.RawMessage{"authority": json.RawMessage("true")}, nil).Complete {
		t.Fatal("affirmative attestation rejected")
	}
	r.Fields[0].Required = false
	if r.Check(map[string]json.RawMessage{"authority": json.RawMessage("true")}, nil).Complete {
		t.Fatal("malformed constraint accepted")
	}
}
