//! Label and configuration constants
//!
//! This module contains all constant values used throughout the labeling system.

/// Common label string constants to ensure consistency across the codebase
pub mod label_constants {
    pub const NONE: &str = "none";
    #[cfg(test)]
    pub const SECRET: &str = "secret";
    pub const PRIVATE_USER: &str = "private:user";
    pub const PRIVATE_BASE: &str = "private";
    pub const READER_PREFIX: &str = "unapproved:";
    pub const WRITER_PREFIX: &str = "approved:";
    pub const MERGED_PREFIX: &str = "merged:";
    pub const NONE_PREFIX: &str = "none:";
    pub const BLOCKED_PREFIX: &str = "blocked:";
    pub const BLOCKED_BASE: &str = "blocked";
    pub const READER_BASE: &str = "unapproved";
    pub const WRITER_BASE: &str = "approved";
    pub const MERGED_BASE: &str = "merged";
    pub const PRIVATE_PREFIX: &str = "private:";
}

/// Canonical policy-facing integrity level tokens.
pub mod policy_integrity {
    pub const NONE: &str = "none";
    pub const UNAPPROVED: &str = "unapproved";
    pub const APPROVED: &str = "approved";
    pub const MERGED: &str = "merged";

    #[cfg(test)]
    pub const ORDER_HIGH_TO_LOW: [&str; 4] = [MERGED, APPROVED, UNAPPROVED, NONE];
    /// Low-to-high order joined with `|`, ready for use in error messages.
    pub const ORDER_LOW_TO_HIGH_PIPED: &str = "none|unapproved|approved|merged";
}

#[cfg(test)]
mod tests {
    use super::{desc_prefix, field_names, policy_integrity};

    /// Ensures ORDER_LOW_TO_HIGH_PIPED stays in sync with ORDER_HIGH_TO_LOW.
    /// If a new integrity level is added or reordered, this test will catch the drift.
    #[test]
    fn order_low_to_high_piped_matches_order_high_to_low() {
        let derived: String = policy_integrity::ORDER_HIGH_TO_LOW
            .iter()
            .rev()
            .copied()
            .collect::<Vec<_>>()
            .join("|");
        assert_eq!(
            derived,
            policy_integrity::ORDER_LOW_TO_HIGH_PIPED,
            "ORDER_LOW_TO_HIGH_PIPED is out of sync with ORDER_HIGH_TO_LOW"
        );
    }

    #[test]
    fn description_prefixes_match_canonical_values() {
        assert_eq!(desc_prefix::REPO, "repo:");
        assert_eq!(desc_prefix::PR, "pr:");
        assert_eq!(desc_prefix::ISSUE, "issue:");
        assert_eq!(desc_prefix::COMMIT, "commit:");
        assert_eq!(desc_prefix::RELEASE, "release:");
        assert_eq!(desc_prefix::GIST, "gist:");
        assert_eq!(desc_prefix::NOTIFICATION, "notification:");
        assert_eq!(field_names::METHOD, "method");
        assert_eq!(field_names::IS_ERROR, "isError");
        assert_eq!(field_names::PUBLIC, "public");
    }
}

/// Canonical *reserved* scope token strings used for baseline and integrity scoping.
///
/// These are the three well-known, fixed scope tokens that represent broad resource
/// categories (org-level, user-level, and cross-repo). Other scopes exist at runtime
/// (e.g. `owner` or `owner/repo` for concrete repositories) — those are constructed
/// dynamically and are not represented here.
/// Using constants avoids silent typos (e.g. "Github") that produce wrong DIFC labels
/// with no compiler error.
pub mod scope_names {
    /// Owner-scoped policy (GitHub-org-level resources)
    pub const GITHUB: &str = "github";
    /// User-scoped policy (personal resources)
    pub const USER: &str = "user";
    /// Global-scoped policy (cross-repo / no specific owner)
    pub const GLOBAL: &str = "global";
}

/// Field name constants for JSON extraction
pub mod field_names {
    pub const OWNER: &str = "owner";
    pub const REPO: &str = "repo";
    pub const ISSUE_NUMBER: &str = "issue_number";
    pub const PULL_NUMBER: &str = "pull_number";
    pub const SHA: &str = "sha";
    pub const MERGED_AT: &str = "merged_at";
    pub const MERGED: &str = "merged";
    pub const METHOD: &str = "method";
    // Commonly accessed response fields
    pub const FULL_NAME: &str = "full_name";
    pub const FULL_NAME_CAMEL: &str = "fullName";
    pub const NUMBER: &str = "number";
    pub const PUBLIC: &str = "public";
    pub const PRIVATE: &str = "private";
    pub const IS_PRIVATE: &str = "is_private";
    pub const IS_PRIVATE_CAMEL: &str = "isPrivate";
    pub const AUTHOR_ASSOCIATION: &str = "author_association";
    pub const AUTHOR_ASSOCIATION_CAMEL: &str = "authorAssociation";
    pub const LOGIN: &str = "login";
    pub const IS_ERROR: &str = "isError";
}

/// Canonical repo `visibility` field string values, used to avoid silent
/// typos when matching against the visibility string returned by the API.
pub mod visibility_values {
    pub const PRIVATE: &str = "private";
    pub const INTERNAL: &str = "internal";
    pub const PUBLIC: &str = "public";
}

/// Canonical description prefix strings used in `ResourceLabels::description`.
/// Using constants prevents silent typos that produce wrong DIFC descriptions.
pub mod desc_prefix {
    pub const REPO: &str = "repo:";
    pub const PR: &str = "pr:";
    pub const ISSUE: &str = "issue:";
    pub const COMMIT: &str = "commit:";
    pub const RELEASE: &str = "release:";
    pub const GIST: &str = "gist:";
    pub const NOTIFICATION: &str = "notification:";
}

/// Sensitive file patterns for detecting secret-containing files
pub const SENSITIVE_FILE_PATTERNS: &[&str] = &[
    ".env",
    ".key",
    ".pem",
    ".p12",
    ".pfx",
    "id_rsa",
    "id_dsa",
    "id_ecdsa",
    "id_ed25519",
];

/// Sensitive keywords in filenames
pub const SENSITIVE_FILE_KEYWORDS: &[&str] = &["secret", "credential", "password", "token"];

/// Buffer size constants for backend calls
pub const SMALL_BUFFER_SIZE: usize = 256 * 1024; // 256KB
pub const MEDIUM_BUFFER_SIZE: usize = 512 * 1024; // 512KB

/// Maximum items to process per response to prevent WASM memory exhaustion
pub const MAX_ITEMS_PER_RESPONSE: usize = 100;

/// Canonical tool-name strings for the granular `*_read` sub-tools and their
/// non-granular legacy counterparts. Centralizing these prevents a typo in
/// any one copy (e.g. `"pull_requests_read"`) from silently breaking a
/// match arm or the `is_non_get_read_sub_method` skip-check, since these
/// are plain `&str` comparisons with no compiler-checked exhaustiveness.
pub mod tool_names {
    pub const PULL_REQUEST_READ: &str = "pull_request_read";
    pub const GET_PULL_REQUEST: &str = "get_pull_request";
    pub const ISSUE_READ: &str = "issue_read";
    pub const GET_ISSUE: &str = "get_issue";
}
