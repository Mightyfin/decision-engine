package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s Server) commercialReview(w http.ResponseWriter, r *http.Request) {
	role := "decision_workload"
	if r.Method == "POST" {
		role = "credit_commercial_reviewer"
	}
	p, ok := s.principal(w, r, role)
	if !ok {
		return
	}
	if p.ApplicationID == "" || p.Subject == "" {
		write(w, 403, map[string]string{"error": "workload_required"})
		return
	}
	a, ok := s.tenantApplication(w, r, p)
	if !ok {
		return
	}
	store, ok := s.Applications.(creditrisk.CommercialReviewStore)
	if !ok {
		write(w, 503, map[string]string{"error": "commercial_review_unavailable"})
		return
	}
	scope := creditrisk.SubmissionIdentity{TenantID: p.TenantID, Environment: p.Environment, CallerApplicationID: p.ApplicationID}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		out, err := store.CommercialReview(r.Context(), a.ID, scope)
		if err != nil {
			commercialError(w, err)
			return
		}
		write(w, 200, out)
		return
	}
	var body struct {
		Revision     int    `json:"revision"`
		SnapshotHash string `json:"snapshot_hash"`
		Decision     string `json:"decision"`
		Reference    string `json:"review_reference"`
		Reason       string `json:"reason"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	scope.Key = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if dec.Decode(&body) != nil || dec.Decode(&struct{}{}) != io.EOF || body.Revision < 1 || len(body.SnapshotHash) != 64 || (body.Decision != "approved" && body.Decision != "rejected") || strings.TrimSpace(body.Reference) == "" || len(body.Reference) > 256 || len(strings.TrimSpace(body.Reason)) < 10 || len(body.Reason) > 4000 || len(scope.Key) < 16 || len(scope.Key) > 128 {
		write(w, 400, map[string]string{"error": "invalid_commercial_review"})
		return
	}
	if _, err := hex.DecodeString(body.SnapshotHash); err != nil {
		write(w, 400, map[string]string{"error": "invalid_commercial_review"})
		return
	}
	raw, _ := json.Marshal([]any{a.ID, body})
	sum := sha256.Sum256(raw)
	scope.Hash = hex.EncodeToString(sum[:])
	out, replay, err := store.RecordCommercialReview(r.Context(), creditrisk.CommercialReview{ApplicationID: a.ID, Revision: body.Revision, SnapshotHash: body.SnapshotHash, Decision: body.Decision, Reference: body.Reference}, creditrisk.Audit{ApplicationID: a.ID, Actor: p.Subject, Reason: body.Reason, At: time.Now().UTC()}, scope)
	if err != nil {
		commercialError(w, err)
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	write(w, 200, out)
}
func commercialError(w http.ResponseWriter, err error) {
	status, code := 503, "commercial_review_unavailable"
	if errors.Is(err, creditrisk.ErrInvalidState) {
		status, code = 409, "commercial_review_conflict"
	}
	if errors.Is(err, creditrisk.ErrDraftKeyConflict) {
		status, code = 409, "idempotency_conflict"
	}
	if errors.Is(err, creditrisk.ErrNotFound) {
		status, code = 404, "not_found"
	}
	write(w, status, map[string]string{"error": code})
}
