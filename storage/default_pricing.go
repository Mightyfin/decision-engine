package storage

import (
	"context"
	"fmt"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s Postgres) BindDefaultPricing(ctx context.Context, tenant, product, actor string, b pricing.DefaultBinding) error {
	if strings.TrimSpace(tenant) == "" || tenant == pricing.DefaultOwner || strings.TrimSpace(product) == "" || strings.TrimSpace(actor) == "" {
		return fmt.Errorf("invalid binding")
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pricing_policies WHERE tenant_id=$1 AND product_policy_id=$2 AND configured_currency=$3 AND active=true)`, pricing.DefaultOwner, b.DefaultPolicyKey, b.Currency).Scan(&available); err != nil {
			return err
		}
		if !available {
			return fmt.Errorf("default pricing not configured for currency")
		}
		_, err := tx.Exec(ctx, `INSERT INTO credit_default_pricing_bindings(tenant_id,product_policy_id,default_policy_key,currency,created_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,product_policy_id) DO UPDATE SET default_policy_key=EXCLUDED.default_policy_key,currency=EXCLUDED.currency,created_by=EXCLUDED.created_by,created_at=now()`, tenant, product, b.DefaultPolicyKey, b.Currency, actor)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO credit_pricing_configuration_audit(tenant_id,product_policy_id,actor,action,configuration) VALUES($1,$2,$3,'bind_default',$4)`, tenant, product, actor, b)
		return err
	})
}
