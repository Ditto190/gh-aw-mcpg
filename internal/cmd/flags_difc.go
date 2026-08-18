package cmd

// DIFC (Decentralized Information Flow Control) related flags

import (
	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/envutil"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/spf13/cobra"
)

// DIFC flag variables
var (
	difcMode          string
	difcSinkServerIDs string // Comma-separated server IDs that should include DIFC tag snapshots in RPC JSONL logs
	guardPolicyJSON   string
	allowOnlyPublic   bool
	allowOnlyOwner    string
	allowOnlyRepo     string
	allowOnlyMinInt   string
)

// registerGuardsModeFlag registers the --guards-mode flag and its completion
// on cmd, storing the value in target. Both the serve (root) command and the
// proxy subcommand share this helper to keep flag description, default, and
// completion in one place.
func registerGuardsModeFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "guards-mode", difc.DefaultEnforcementMode(),
		"Guards enforcement mode: strict (deny violations), filter (remove denied tools), or propagate (auto-adjust agent labels on reads)")
	registerGuardsModeCompletion(cmd)
}

// registerGuardsModeCompletion registers shell completion for --guards-mode.
// Split out from registerGuardsModeFlag so the completion-registration
// failure path (e.g. duplicate registration) can be exercised in isolation
// without re-declaring the underlying flag.
func registerGuardsModeCompletion(cmd *cobra.Command) {
	if err := cmd.RegisterFlagCompletionFunc("guards-mode", cobra.FixedCompletions(
		difc.ValidModes, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		debugLog.Printf("Failed to register completion for --guards-mode: %v", err)
	}
}

func registerAllowOnlyScopeFlags(cmd *cobra.Command, public *bool, owner, repo, minIntegrity *string) {
	cmd.Flags().BoolVar(public, "allowonly-scope-public", envutil.GetEnvBool(config.EnvAllowOnlyScopePublic, false), "Use public AllowOnly scope")
	cmd.Flags().StringVar(owner, "allowonly-scope-owner", envutil.GetEnvString(config.EnvAllowOnlyScopeOwner, ""), "AllowOnly owner scope value")
	cmd.Flags().StringVar(repo, "allowonly-scope-repo", envutil.GetEnvString(config.EnvAllowOnlyScopeRepo, ""), "AllowOnly repo name (requires owner)")
	cmd.Flags().StringVar(minIntegrity, "allowonly-min-integrity", envutil.GetEnvString(config.EnvAllowOnlyMinIntegrity, ""), "AllowOnly integrity: none|unapproved|approved|merged")

	// Cobra validates flags that were explicitly changed, rather than their
	// resolved values. When either value has an environment-derived default,
	// leave value-based validation to BuildAllowOnlyPolicy so CLI flags can
	// clear that default while selecting the other scope.
	if !envutil.HasEnvVar(config.EnvAllowOnlyScopePublic) && !envutil.HasEnvVar(config.EnvAllowOnlyScopeOwner) {
		cmd.MarkFlagsMutuallyExclusive("allowonly-scope-public", "allowonly-scope-owner")
	}
}

func init() {
	RegisterFlag(func(cmd *cobra.Command) {
		registerGuardsModeFlag(cmd, &difcMode)
		cmd.Flags().StringVar(&difcSinkServerIDs, "guards-sink-server-ids", envutil.GetEnvString("MCP_GATEWAY_GUARDS_SINK_SERVER_IDS", ""), "Comma-separated server IDs whose RPC JSONL logs should include agent secrecy/integrity tag snapshots")
		cmd.Flags().StringVar(&guardPolicyJSON, "guard-policy-json", envutil.GetEnvString(config.EnvGuardPolicyJSON, ""), "Guard policy JSON (e.g. {\"allow-only\":{\"repos\":\"public\",\"min-integrity\":\"none\"}})")
		registerAllowOnlyScopeFlags(cmd, &allowOnlyPublic, &allowOnlyOwner, &allowOnlyRepo, &allowOnlyMinInt)
	})
}

// detectGuardWasm returns the path to the WASM guard module to use as the
// default for the --guard-wasm flag. It checks in order:
//  1. The baked-in container guard at guard.ContainerGuardWasmPath.
//  2. The first .wasm file under $MCP_GATEWAY_WASM_GUARDS_DIR/github/.
//
// Returns an empty string when no guard can be auto-detected, which causes
// --guard-wasm to be marked as required.
func detectGuardWasm() string {
	if wasmPath, found, err := guard.FindGuardFile("github"); err != nil {
		debugLog.Printf("WASM guard discovery failed: %v", err)
	} else if found {
		debugLog.Printf("Auto-detected guard: %s", wasmPath)
		return wasmPath
	}

	debugLog.Print("Baked-in guard not found, --guard-wasm flag required")
	return ""
}

func resolveGuardPolicyFromFlags(cmd *cobra.Command) (*config.GuardPolicy, string, error) {
	cliGuardPolicyChanged := cmd.Flags().Changed("guard-policy-json")
	cliChanged := cliGuardPolicyChanged ||
		cmd.Flags().Changed("allowonly-scope-public") ||
		cmd.Flags().Changed("allowonly-scope-owner") ||
		cmd.Flags().Changed("allowonly-scope-repo") ||
		cmd.Flags().Changed("allowonly-min-integrity")

	debugLog.Printf("Resolving guard policy override: cliChanged=%v, cliGuardPolicyChanged=%v, allowOnlyPublic=%v, owner=%q, repo=%q, minIntegrity=%q",
		cliChanged, cliGuardPolicyChanged, allowOnlyPublic, allowOnlyOwner, allowOnlyRepo, allowOnlyMinInt)

	cliPolicyJSON := ""
	if cliGuardPolicyChanged {
		cliPolicyJSON = guardPolicyJSON
		debugLog.Printf("Using CLI guard-policy-json: len=%d", len(cliPolicyJSON))
	}

	policy, source, err := config.ResolveGuardPolicyOverride(
		cliChanged,
		cliPolicyJSON,
		allowOnlyPublic,
		allowOnlyOwner,
		allowOnlyRepo,
		allowOnlyMinInt,
	)
	if err != nil {
		debugLog.Printf("Guard policy resolution failed: %v", err)
	} else if policy != nil {
		debugLog.Printf("Guard policy resolved: source=%q", source)
	} else {
		debugLog.Print("No guard policy override configured")
	}
	return policy, source, err
}
