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
	ID, TenantID, Code, Currency     string
	Version                          int
	MinimumAmount, MaximumAmount     int64 // minor units
	MinimumTermDays, MaximumTermDays int
	Active                           bool
}

type Store interface {
	Policy(context.Context, string, string) (Policy, error)
}
type Service struct{ Store Store }

func (s Service) Validate(ctx context.Context, tenantID, policyID, currency string, amount int64, termDays int) (Policy, error) {
	p, err := s.Store.Policy(ctx, tenantID, policyID)
	if err != nil {
		return Policy{}, err
	}
	if !p.Active || p.Version < 1 || strings.TrimSpace(currency) != p.Currency || amount < p.MinimumAmount || amount > p.MaximumAmount || termDays < p.MinimumTermDays || termDays > p.MaximumTermDays {
		return Policy{}, fmt.Errorf("request does not meet configured product policy")
	}
	return p, nil
}
