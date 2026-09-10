package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("product policy not found")

// Policy is the product-engine output. It has no payment or accounting behaviour.
type Policy struct {
	UsageTerms                       json.RawMessage `json:"usage_terms,omitempty"`
	ID, TenantID, Code, Currency     string
	Version                          int
	MinimumAmount, MaximumAmount     int64 // minor units
	MinimumTermDays, MaximumTermDays int
	RepaymentIntervalDays, GraceDays int
	AllocationOrder                  []string
	AllowedApplicantRoles            []string
	RequiredDocumentTypes            []string
	ApplicationRequirements          map[string]Requirements
	Active                           bool
}

type Store interface {
	Policy(context.Context, string, string) (Policy, error)
}
type Service struct{ Store Store }

func (s Service) Validate(ctx context.Context, tenantID, policyID, currency string, amount int64, termDays int, applicantRole ...string) (Policy, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(policyID) == "" {
		return Policy{}, ErrNotFound
	}
	p, err := s.Store.Policy(ctx, tenantID, policyID)
	if err != nil {
		return Policy{}, err
	}
	// All adapters must satisfy the same ownership boundary. A cached or local
	// store must not be able to substitute another tenant's policy.
	if p.TenantID != tenantID || p.ID != policyID {
		return Policy{}, ErrNotFound
	}
	if !supportsUsage(p.UsageTerms) {
		return Policy{}, ErrNotFound
	}
	// Reject malformed configuration rather than allowing invalid ranges to
	// authorize a request. Amounts remain integer minor units; no defaults here.
	if p.MinimumAmount <= 0 || p.MaximumAmount < p.MinimumAmount || p.MinimumTermDays <= 0 || p.MaximumTermDays < p.MinimumTermDays || len(p.Currency) != 3 || strings.Trim(p.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return Policy{}, fmt.Errorf("invalid configured product policy")
	}
	if !p.Active || p.Version < 1 || strings.TrimSpace(currency) != p.Currency || amount < p.MinimumAmount || amount > p.MaximumAmount || termDays < p.MinimumTermDays || termDays > p.MaximumTermDays || p.RepaymentIntervalDays < 1 || p.GraceDays < 0 || !ValidAllocationOrder(p.AllocationOrder) {
		return Policy{}, fmt.Errorf("request does not meet configured product policy")
	}
	if len(p.AllowedApplicantRoles) > 0 {
		role := ""
		if len(applicantRole) > 0 {
			role = strings.TrimSpace(applicantRole[0])
		}
		allowed := false
		for _, configured := range p.AllowedApplicantRoles {
			if configured == role {
				allowed = true
				break
			}
		}
		if !allowed {
			return Policy{}, fmt.Errorf("applicant role does not meet configured product policy")
		}
	}
	return p, nil
}

func ValidAllocationOrder(order []string) bool {
	if len(order) != 4 {
		return false
	}
	seen := map[string]bool{}
	for _, component := range order {
		if component != "principal" && component != "interest" && component != "fees" && component != "penalty" || seen[component] {
			return false
		}
		seen[component] = true
	}
	return true
}
