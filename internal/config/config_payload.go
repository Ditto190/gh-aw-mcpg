// Package config provides configuration loading and parsing.
// This file defines payload-related configuration and defaults.
package config

// DefaultPayloadDir is the default directory for storing large payloads.
const DefaultPayloadDir = "/tmp/jq-payloads"

// DefaultPayloadSizeThreshold is the default size threshold (in bytes) for storing payloads to disk.
// Payloads larger than this threshold are stored to disk, smaller ones are returned inline.
// Default: 524288 bytes (512KB) - chosen to accommodate typical MCP tool responses including
// GitHub API queries (list_commits, list_issues, etc.) without triggering disk storage.
// This prevents agent looping issues when payloadPath is not accessible in agent containers.
const DefaultPayloadSizeThreshold = 524288

func init() {
	// Register default setter for PayloadDir and PayloadSizeThreshold
	RegisterDefaults(func(cfg *Config) {
		if cfg.Gateway != nil {
			if cfg.Gateway.PayloadDir == "" {
				cfg.Gateway.PayloadDir = DefaultPayloadDir
			}
			if cfg.Gateway.PayloadSizeThreshold == 0 {
				cfg.Gateway.PayloadSizeThreshold = DefaultPayloadSizeThreshold
			}
		}
	})
}
