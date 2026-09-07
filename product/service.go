package product

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("product policy not found")

// Policy is the product-engine output. It has no payment or accounting behaviour.
type Policy struct {
	ID, TenantID, Code, Currency, PricingPolicyID string
	Version                                       int
	MinimumAmount, MaximumAmount                  int64 // minor units
	MinimumTermDays, MaximumTermDays              int
	RepaymentIntervalDays, GraceDays              int
	AllocationOrder                               []string
	AllowedApplicantRoles                         []string
	Active                                        bool
}

type Store interface {
	Policy(context.Context, string, string) (Policy, error)
}
type Service struct{ Store Store }

func (s Service) Validate(ctx context.Context, tenantID, policyID, currency string, amount int64, termDays int, applicantRole ...string) (Policy, error) {
	p, err := s.Store.Policy(ctx, tenantID, policyID)
	if err != nil {
		return Policy{}, err
	}
	if !p.Active || p.Version < 1 || p.PricingPolicyID == "" || strings.TrimSpace(currency) != p.Currency || amount < p.MinimumAmount || amount > p.MaximumAmount || termDays < p.MinimumTermDays || termDays > p.MaximumTermDays || p.RepaymentIntervalDays < 1 || p.GraceDays < 0 || !ValidAllocationOrder(p.AllocationOrder) {
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
