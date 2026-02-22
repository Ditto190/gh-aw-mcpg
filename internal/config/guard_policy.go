package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MinIntegrityNone    = "none"
	MinIntegrityUnapproved = "unapproved"
	MinIntegrityApproved    = "approved"
	MinIntegrityMerged        = "merged"
)

var validMinIntegrityValues = map[string]struct{}{
	MinIntegrityNone:    {},
	MinIntegrityUnapproved: {},
	MinIntegrityApproved: {},
	MinIntegrityMerged:        {},
}

// GuardPolicy represents the policy payload passed to guard label_agent.
type GuardPolicy struct {
	AllowOnly *AllowOnlyPolicy `toml:"AllowOnly" json:"AllowOnly,omitempty"`
}

// AllowOnlyPolicy configures scope and minimum required integrity.
type AllowOnlyPolicy struct {
	Scope        interface{} `toml:"Scope" json:"Scope"`
	MinIntegrity string      `toml:"MinIntegrity" json:"MinIntegrity"`
}

// NormalizedGuardPolicy is a canonical policy representation for caching and observability.
type NormalizedGuardPolicy struct {
	ScopeKind    string `json:"scope_kind"`
	ScopeOwner   string `json:"scope_owner,omitempty"`
	ScopeRepo    string `json:"scope_repo,omitempty"`
	MinIntegrity string `json:"min_integrity"`
}

// ValidateGuardPolicy validates AllowOnly policy input.
func ValidateGuardPolicy(policy *GuardPolicy) error {
	_, err := NormalizeGuardPolicy(policy)
	return err
}

// NormalizeGuardPolicy validates and normalizes policy shape.
func NormalizeGuardPolicy(policy *GuardPolicy) (*NormalizedGuardPolicy, error) {
	if policy == nil || policy.AllowOnly == nil {
		return nil, fmt.Errorf("policy must include AllowOnly")
	}

	minIntegrity := strings.ToLower(strings.TrimSpace(policy.AllowOnly.MinIntegrity))
	if _, ok := validMinIntegrityValues[minIntegrity]; !ok {
		return nil, fmt.Errorf("AllowOnly.MinIntegrity must be one of: none, unapproved, approved, merged")
	}

	normalized := &NormalizedGuardPolicy{MinIntegrity: minIntegrity}

	switch scope := policy.AllowOnly.Scope.(type) {
	case string:
		if strings.TrimSpace(scope) != "public" {
			return nil, fmt.Errorf("AllowOnly.Scope string must be 'public'")
		}
		normalized.ScopeKind = "public"
		return normalized, nil

	case map[string]interface{}:
		owner, ownerOK := scope["owner"].(string)
		repo, repoOK := scope["repo"].(string)
		owner = strings.TrimSpace(owner)
		repo = strings.TrimSpace(repo)

		if repoOK && !ownerOK {
			return nil, fmt.Errorf("AllowOnly.Scope repo requires owner")
		}
		if ownerOK && owner == "" {
			return nil, fmt.Errorf("AllowOnly.Scope owner must not be empty")
		}
		if repoOK && repo == "" {
			return nil, fmt.Errorf("AllowOnly.Scope repo must not be empty")
		}

		if repoOK {
			normalized.ScopeKind = "repo"
			normalized.ScopeOwner = owner
			normalized.ScopeRepo = repo
			return normalized, nil
		}
		if ownerOK {
			normalized.ScopeKind = "owner"
			normalized.ScopeOwner = owner
			return normalized, nil
		}
		return nil, fmt.Errorf("AllowOnly.Scope object must include owner, or owner+repo")

	default:
		return nil, fmt.Errorf("AllowOnly.Scope must be 'public' or an object with owner[/repo]")
	}
}

// ParseGuardPolicyJSON parses policy JSON and validates it.
func ParseGuardPolicyJSON(policyJSON string) (*GuardPolicy, error) {
	policy := &GuardPolicy{}
	if err := json.Unmarshal([]byte(policyJSON), policy); err != nil {
		return nil, fmt.Errorf("invalid guard policy JSON: %w", err)
	}
	if err := ValidateGuardPolicy(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func validateGuardPolicies(cfg *Config) error {
	for name, guardCfg := range cfg.Guards {
		if guardCfg != nil && guardCfg.Policy != nil {
			if err := ValidateGuardPolicy(guardCfg.Policy); err != nil {
				return fmt.Errorf("invalid policy for guard '%s': %w", name, err)
			}
		}
	}
	return nil
}
