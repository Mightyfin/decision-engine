package pricing

// DefaultOwner is an internal configuration namespace, never a customer tenant.
const DefaultOwner = "__mightyfin_default__"

type DefaultBinding struct {
	DefaultPolicyKey string `json:"default_policy_key"`
	Currency         string `json:"currency"`
}
