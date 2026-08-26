package enclavegithub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logPolicy = logger.ForFile()

const (
	ProfileIssuesReadV1 = "issues-read-v1"
	DefaultAudience     = "gh-aw-enclave-github"
	CapabilityPrefix    = "awf-egh1"

	EnvCapabilityKey = "MCP_GATEWAY_ENCLAVE_CAPABILITY_KEY"
	EnvPolicyJSON    = "MCP_GATEWAY_ENCLAVE_POLICY_JSON"

	OperationIssueCommentsList  = "issues.comments.list"
	OperationIssuesGet          = "issues.get"
	OperationIssuesList         = "issues.list"
	MaxCapabilityTTLSeconds     = 600
	maxPolicyJSONBytes          = 64 * 1024
	maxWorkflowRunIdentityBytes = 64
)

var (
	repositoryPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_.-]{0,38})/[a-z0-9_.-]{1,100}$`)
	runIdentityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	invocationPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$`)
	validOperations    = map[string]struct{}{
		OperationIssuesList:        {},
		OperationIssuesGet:         {},
		OperationIssueCommentsList: {},
	}
	validSensitivities = map[string]struct{}{
		"public":       {},
		"internal":     {},
		"confidential": {},
		"sealed":       {},
	}
	validIntegrityLevels = map[string]struct{}{
		"none":       {},
		"unapproved": {},
		"approved":   {},
		"merged":     {},
	}
)

// RepositoryPolicy identifies a repository assignable to an enclave invocation.
type RepositoryPolicy struct {
	Repo        string `json:"repo"`
	Sensitivity string `json:"sensitivity"`
}

// Policy is the compiler-owned authorization contract for one workflow run.
type Policy struct {
	Version                 int                `json:"version"`
	Profile                 string             `json:"profile"`
	Audience                string             `json:"audience"`
	WorkflowRunID           string             `json:"workflow_run_id"`
	Repositories            []RepositoryPolicy `json:"repositories"`
	PublicMinIntegrity      string             `json:"public_min_integrity"`
	AllowedOperations       []string           `json:"allowed_operations"`
	MaxCapabilityTTLSeconds int64              `json:"max_capability_ttl_seconds"`
}

// ParsePolicy parses and validates a compiler-generated enclave policy.
func ParsePolicy(raw string) (*Policy, error) {
	logPolicy.Printf("Parsing enclave policy: %d bytes", len(raw))
	if len(raw) == 0 {
		return nil, fmt.Errorf("enclave policy is required")
	}
	if len(raw) > maxPolicyJSONBytes {
		logPolicy.Printf("Enclave policy rejected: %d bytes exceeds limit of %d bytes", len(raw), maxPolicyJSONBytes)
		return nil, fmt.Errorf("enclave policy exceeds %d bytes", maxPolicyJSONBytes)
	}

	var policy Policy
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return nil, fmt.Errorf("invalid enclave policy JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		logPolicy.Printf("Enclave policy validation failed for profile %q: %v", policy.Profile, err)
		return nil, err
	}
	logPolicy.Printf("Enclave policy parsed successfully: profile=%s workflow_run_id=%s repositories=%d", policy.Profile, policy.WorkflowRunID, len(policy.Repositories))
	return &policy, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("enclave policy must contain exactly one JSON value")
		}
		return fmt.Errorf("invalid enclave policy JSON: %w", err)
	}
	return nil
}

// Validate checks policy invariants and canonicalizes set-like fields.
func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("enclave policy is required")
	}
	if p.Version != 1 {
		return fmt.Errorf("enclave policy version must be 1")
	}
	if p.Profile != ProfileIssuesReadV1 {
		return fmt.Errorf("unsupported enclave profile %q", p.Profile)
	}
	if p.Audience != DefaultAudience {
		return fmt.Errorf("unsupported enclave audience %q", p.Audience)
	}
	if len(p.WorkflowRunID) > maxWorkflowRunIdentityBytes ||
		!runIdentityPattern.MatchString(p.WorkflowRunID) {
		return fmt.Errorf("workflow_run_id must be a canonical value no longer than %d bytes", maxWorkflowRunIdentityBytes)
	}
	if len(p.Repositories) == 0 {
		return fmt.Errorf("repositories must contain at least one entry")
	}

	seenRepos := make(map[string]struct{}, len(p.Repositories))
	for i := range p.Repositories {
		repo := &p.Repositories[i]
		if !repositoryPattern.MatchString(repo.Repo) {
			return fmt.Errorf("repositories[%d].repo must be canonical lowercase owner/name", i)
		}
		if _, exists := seenRepos[repo.Repo]; exists {
			return fmt.Errorf("repositories must not contain duplicate repo %q", repo.Repo)
		}
		seenRepos[repo.Repo] = struct{}{}
		if _, ok := validSensitivities[repo.Sensitivity]; !ok {
			return fmt.Errorf("repositories[%d].sensitivity is invalid", i)
		}
	}

	if _, ok := validIntegrityLevels[p.PublicMinIntegrity]; !ok {
		return fmt.Errorf("public_min_integrity must be one of none, unapproved, approved, merged")
	}
	if len(p.AllowedOperations) == 0 {
		return fmt.Errorf("allowed_operations must contain at least one operation")
	}
	if !slices.IsSorted(p.AllowedOperations) {
		return fmt.Errorf("allowed_operations must be lexicographically sorted")
	}
	seenOps := make(map[string]struct{}, len(p.AllowedOperations))
	for _, operation := range p.AllowedOperations {
		if _, ok := validOperations[operation]; !ok {
			return fmt.Errorf("unsupported allowed operation %q", operation)
		}
		if _, exists := seenOps[operation]; exists {
			return fmt.Errorf("allowed_operations must not contain duplicate operation %q", operation)
		}
		seenOps[operation] = struct{}{}
	}
	if p.MaxCapabilityTTLSeconds <= 0 || p.MaxCapabilityTTLSeconds > MaxCapabilityTTLSeconds {
		return fmt.Errorf("max_capability_ttl_seconds must be between 1 and %d", MaxCapabilityTTLSeconds)
	}
	return nil
}

// HasRepository reports whether repo is an assignable repository in the policy.
func (p *Policy) HasRepository(repo string) bool {
	return slices.ContainsFunc(p.Repositories, func(candidate RepositoryPolicy) bool {
		return candidate.Repo == repo
	})
}

// RepositorySensitivity returns the configured sensitivity for repo.
func (p *Policy) RepositorySensitivity(repo string) (string, bool) {
	for _, candidate := range p.Repositories {
		if candidate.Repo == repo {
			return candidate.Sensitivity, true
		}
	}
	return "", false
}

// NormalizeRepository returns a canonical lowercase owner/name repository.
func NormalizeRepository(repo string) (string, bool) {
	normalized := strings.ToLower(repo)
	return normalized, repositoryPattern.MatchString(normalized)
}

// AllowsOperation reports whether operation is enabled by the compiler policy.
func (p *Policy) AllowsOperation(operation string) bool {
	return slices.Contains(p.AllowedOperations, operation)
}

// GuardRepos returns exact assigned repositories followed by the public catch-all.
func (p *Policy) GuardRepos() []string {
	repos := make([]string, 0, len(p.Repositories)+1)
	for _, repository := range p.Repositories {
		repos = append(repos, repository.Repo)
	}
	repos = append(repos, "public")
	return repos
}

// GuardPolicyJSON returns the internal allow-only policy for this enclave profile.
func (p *Policy) GuardPolicyJSON() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	repos := p.GuardRepos()
	logPolicy.Printf("Generating guard policy JSON: repos=%d min_integrity=%s", len(repos), p.PublicMinIntegrity)
	policy := map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         repos,
			"min-integrity": p.PublicMinIntegrity,
		},
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("failed to encode enclave guard policy: %w", err)
	}
	return string(encoded), nil
}
