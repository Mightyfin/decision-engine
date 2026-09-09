package storage

import (
	"context"
	"encoding/json"
	"errors"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/jackc/pgx/v5"
)

func (s Postgres) ReviewQuestionnaire(ctx context.Context, id, tenant, environment string) (*creditrisk.Questionnaire, error) {
	if tenant == "" || environment == "" {
		return nil, creditrisk.ErrEvidenceScope
	}
	var q creditrisk.Questionnaire
	var raw, answers []byte
	var revision *int
	err := s.Pool.QueryRow(ctx, `SELECT d.requirements,d.answers,d.revision,a.product_policy_version
	FROM credit_applications a LEFT JOIN credit_application_drafts d ON d.application_id=a.id
	LEFT JOIN credit_application_environments e ON e.application_id=a.id
	WHERE a.id=$1 AND a.tenant_id=$2 AND (d.application_id IS NULL OR e.environment=$3)`, id, tenant, environment).Scan(&raw, &answers, &revision, &q.ProductVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, creditrisk.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revision == nil {
		return nil, nil
	}
	q.Revision = *revision
	var requirements product.Requirements
	if err = json.Unmarshal(raw, &requirements); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(answers, &q.Answers); err != nil {
		return nil, err
	}
	q.Fields = requirements.Fields
	q.RequiredDocumentTypes = requirements.RequiredDocumentTypes
	return &q, nil
}
