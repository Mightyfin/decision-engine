// Package migrations applies the Decision Engine schema in release order.
package migrations

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 00001_decision_engine.sql
var initial string

//go:embed 00002_credit_outbox.sql
var creditOutbox string

//go:embed 00003_credit_outbox_delivery.sql
var creditOutboxDelivery string

//go:embed 00004_credit_applicant_subject.sql
var creditApplicantSubject string

//go:embed 00005_servicing_terms.sql
var servicingTerms string

//go:embed 00006_dynamic_origin_tags.sql
var dynamicOriginTags string

//go:embed 00007_external_product_authority.sql
var externalProductAuthority string

//go:embed 00008_pricing_methods.sql
var pricingMethods string

//go:embed 00009_application_evidence.sql
var applicationEvidence string

//go:embed 00010_information_requests.sql
var informationRequests string

//go:embed 00011_application_drafts.sql
var applicationDrafts string

//go:embed 00012_tenant_discovery.sql
var tenantDiscovery string

//go:embed 00013_offer_acceptance_replay.sql
var offerAcceptanceReplay string

//go:embed 00014_submission_replay.sql
var submissionReplay string

//go:embed 00015_application_action_replay.sql
var applicationActionReplay string

//go:embed 00016_submission_ownership.sql
var submissionOwnership string

//go:embed 00017_commercial_review.sql
var commercialReview string

//go:embed 00018_usage_snapshots.sql
var usageSnapshots string

//go:embed 00019_offer_evidence_snapshot.sql
var offerEvidenceSnapshot string

//go:embed 00020_purchase_restrictions.sql
var purchaseRestrictions string

//go:embed 00021_purchase_destination_verification.sql
var purchaseDestinationVerification string

func Up(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS decision_engine_schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00001_decision_engine", initial); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00002_credit_outbox", creditOutbox); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00003_credit_outbox_delivery", creditOutboxDelivery); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00004_credit_applicant_subject", creditApplicantSubject); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00005_servicing_terms", servicingTerms); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00006_dynamic_origin_tags", dynamicOriginTags); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00007_external_product_authority", externalProductAuthority); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00008_pricing_methods", pricingMethods); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00009_application_evidence", applicationEvidence); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00010_information_requests", informationRequests); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00011_application_drafts", applicationDrafts); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00012_tenant_discovery", tenantDiscovery); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00013_offer_acceptance_replay", offerAcceptanceReplay); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00014_submission_replay", submissionReplay); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00015_application_action_replay", applicationActionReplay); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00016_submission_ownership", submissionOwnership); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00017_commercial_review", commercialReview); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00018_usage_snapshots", usageSnapshots); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00019_offer_evidence_snapshot", offerEvidenceSnapshot); err != nil {
		return err
	}
	if err := apply(ctx, pool, "00020_purchase_restrictions", purchaseRestrictions); err != nil {
		return err
	}
	return apply(ctx, pool, "00021_purchase_destination_verification", purchaseDestinationVerification)
}

func apply(ctx context.Context, pool *pgxpool.Pool, version, source string) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM decision_engine_schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	up, _, _ := strings.Cut(source, "-- +goose Down")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, up); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO decision_engine_schema_migrations(version) VALUES($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
