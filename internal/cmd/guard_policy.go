package cmd

import "github.com/spf13/cobra"

import "github.com/github/gh-aw-mcpg/internal/config"

func resolveGuardPolicyOverride(cmd *cobra.Command) (*config.GuardPolicy, string, error) {
	cliChanged := cmd.Flags().Changed("guard-policy-json") ||
		cmd.Flags().Changed("allowonly-scope-public") ||
		cmd.Flags().Changed("allowonly-scope-owner") ||
		cmd.Flags().Changed("allowonly-scope-repo") ||
		cmd.Flags().Changed("allowonly-min-integrity")

	return config.ResolveGuardPolicyOverride(
		cliChanged,
		guardPolicyJSON,
		allowOnlyPublic,
		allowOnlyOwner,
		allowOnlyRepo,
		allowOnlyMinInt,
	)
}
