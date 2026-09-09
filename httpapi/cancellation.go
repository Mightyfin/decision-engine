package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
)

func (s Server) cancelApplication(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r, "credit_application_writer")
	if !ok {
		return
	}
	a, ok := s.tenantApplication(w, r, p)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if dec.Decode(&input) != nil || dec.Decode(&struct{}{}) != io.EOF || len(strings.TrimSpace(input.Reason)) < 10 || len(input.Reason) > 4000 || len(key) < 16 || len(key) > 128 || p.ApplicationID == "" || p.Subject == "" {
		write(w, 400, map[string]string{"error": "reason_application_and_idempotency_key_required"})
		return
	}
	store, ok := s.Applications.(creditrisk.CancellationStore)
	if !ok {
		write(w, 503, map[string]string{"error": "cancellation_unavailable"})
		return
	}
	body, _ := json.Marshal([]string{a.ID, input.Reason})
	digest := sha256.Sum256(body)
	result, replayed, err := store.CancelApplication(r.Context(), a, creditrisk.Audit{ApplicationID: a.ID, Actor: p.Subject, Reason: input.Reason, At: time.Now().UTC()}, creditrisk.SubmissionIdentity{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID, Key: key, Hash: hex.EncodeToString(digest[:])})
	if err != nil {
		status, code := 503, "cancellation_unavailable"
		if errors.Is(err, creditrisk.ErrInvalidState) {
			status, code = 409, "cancellation_not_permitted"
		}
		if errors.Is(err, creditrisk.ErrDraftKeyConflict) {
			status, code = 409, "idempotency_conflict"
		}
		write(w, status, map[string]string{"error": code})
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	write(w, 200, result)
}
