package cmd

import (
	"os"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterGuardsModeFlag(t *testing.T) {
	t.Run("registers flag with default value and completion", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		var mode string
		registerGuardsModeFlag(cmd, &mode)

		flag := cmd.Flags().Lookup("guards-mode")
		require.NotNil(t, flag, "expected --guards-mode flag to be registered")
		assert.Equal(t, difc.DefaultEnforcementMode(), mode, "flag default should match difc.DefaultEnforcementMode()")
		assert.Equal(t, difc.DefaultEnforcementMode(), flag.DefValue)
	})

	t.Run("registers completion successfully", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		var mode string
		cmd.Flags().StringVar(&mode, "guards-mode", difc.DefaultEnforcementMode(), "placeholder")

		assert.NotPanics(t, func() {
			registerGuardsModeCompletion(cmd)
		})
	})

	t.Run("logs but does not panic when completion func already registered", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		var mode string

		// Pre-registering the completion func for --guards-mode before
		// calling registerGuardsModeCompletion forces RegisterFlagCompletionFunc
		// to return the "already registered" error inside
		// registerGuardsModeCompletion, which logs rather than propagates it.
		// Verify that the duplicate registration does not panic.
		assert.NotPanics(t, func() {
			cmd.Flags().StringVar(&mode, "guards-mode", difc.DefaultEnforcementMode(), "placeholder")
			require.NoError(t, cmd.RegisterFlagCompletionFunc("guards-mode", cobra.FixedCompletions(
				difc.ValidModes, cobra.ShellCompDirectiveNoFileComp)))

			registerGuardsModeCompletion(cmd)
		})

		flag := cmd.Flags().Lookup("guards-mode")
		require.NotNil(t, flag)
		assert.Equal(t, difc.DefaultEnforcementMode(), flag.DefValue)
	})
}

func TestValidateDIFCMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{
			name:    "strict mode valid",
			mode:    "strict",
			wantErr: false,
		},
		{
			name:    "filter mode valid",
			mode:    "filter",
			wantErr: false,
		},
		{
			name:    "propagate mode valid",
			mode:    "propagate",
			wantErr: false,
		},
		{
			name:    "uppercase STRICT valid",
			mode:    "STRICT",
			wantErr: false,
		},
		{
			name:    "mixed case Filter valid",
			mode:    "Filter",
			wantErr: false,
		},
		{
			name:    "invalid mode",
			mode:    "invalid",
			wantErr: true,
		},
		{
			name:    "empty mode defaults to strict",
			mode:    "",
			wantErr: false,
		},
		{
			name:    "partial match should fail",
			mode:    "stric",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := difc.ParseEnforcementMode(tt.mode)
			if tt.wantErr {
				assert.Error(t, err, "expected error for mode %q", tt.mode)
			} else {
				assert.NoError(t, err, "unexpected error for mode %q", tt.mode)
			}
		})
	}
}

func TestValidDIFCModes(t *testing.T) {
	require := require.New(t)

	// Verify all expected modes are valid using difc.ParseEnforcementMode
	_, err := difc.ParseEnforcementMode(difc.ModeStrict)
	require.NoError(err, "strict should be valid")
	_, err = difc.ParseEnforcementMode(difc.ModeFilter)
	require.NoError(err, "filter should be valid")
	_, err = difc.ParseEnforcementMode(difc.ModePropagate)
	require.NoError(err, "propagate should be valid")

	// Verify ValidModes slice has 3 entries
	require.Len(difc.ValidModes, 3, "should only have 3 valid modes")
}

func TestParseDIFCSinkServerIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		expect  []string
		wantErr bool
	}{
		{
			name:   "empty input",
			input:  "",
			expect: nil,
		},
		{
			name:   "single server id",
			input:  "safeoutputs",
			expect: []string{"safeoutputs"},
		},
		{
			name:   "multiple server ids",
			input:  "safeoutputs,github",
			expect: []string{"safeoutputs", "github"},
		},
		{
			name:   "trims whitespace around separators",
			input:  " safeoutputs , github ",
			expect: []string{"safeoutputs", "github"},
		},
		{
			name:   "deduplicates server ids",
			input:  "safeoutputs,github,safeoutputs",
			expect: []string{"safeoutputs", "github"},
		},
		{
			name:   "consecutive commas skip empty parts",
			input:  "safeoutputs,,github",
			expect: []string{"safeoutputs", "github"},
		},
		{
			name:   "trailing comma skips empty part",
			input:  "safeoutputs,github,",
			expect: []string{"safeoutputs", "github"},
		},
		{
			name:    "rejects embedded whitespace",
			input:   "safe outputs",
			wantErr: true,
		},
		{
			name:    "rejects embedded tab",
			input:   "safe\toutputs",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := difc.ParseSinkServerIDs(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func setupAllowOnlyScopeCmd(t *testing.T) (*cobra.Command, *bool, *string) {
	t.Helper()
	cmd := &cobra.Command{
		Use:          "test",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	public := new(bool)
	owner := new(string)
	registerAllowOnlyScopeFlags(cmd, public, owner, new(string), new(string))
	return cmd, public, owner
}

func TestAllowOnlyScopeFlagsMutuallyExclusive(t *testing.T) {
	clearEnv := func(t *testing.T) {
		t.Helper()
		for _, name := range []string{config.EnvAllowOnlyScopePublic, config.EnvAllowOnlyScopeOwner} {
			value, wasSet := os.LookupEnv(name)
			require.NoError(t, os.Unsetenv(name))
			t.Cleanup(func() {
				if wasSet {
					require.NoError(t, os.Setenv(name, value))
					return
				}
				require.NoError(t, os.Unsetenv(name))
			})
		}
	}

	t.Run("public and owner together are rejected by cobra", func(t *testing.T) {
		clearEnv(t)
		cmd, _, _ := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{"--allowonly-scope-public", "--allowonly-scope-owner", "octocat"})
		err := cmd.Execute()
		require.Error(t, err, "should fail when both --allowonly-scope-public and --allowonly-scope-owner are provided")
		assert.Contains(t, err.Error(), "allowonly-scope-public", "error should mention allowonly-scope-public")
		assert.Contains(t, err.Error(), "allowonly-scope-owner", "error should mention allowonly-scope-owner")
	})

	t.Run("public alone is accepted", func(t *testing.T) {
		clearEnv(t)
		cmd, _, _ := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{"--allowonly-scope-public"})
		err := cmd.Execute()
		require.NoError(t, err, "should succeed when only --allowonly-scope-public is provided")
	})

	t.Run("owner alone is accepted", func(t *testing.T) {
		clearEnv(t)
		cmd, _, _ := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{"--allowonly-scope-owner", "octocat"})
		err := cmd.Execute()
		require.NoError(t, err, "should succeed when only --allowonly-scope-owner is provided")
	})

	t.Run("neither flag is accepted", func(t *testing.T) {
		clearEnv(t)
		cmd, _, _ := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.NoError(t, err, "should succeed when neither AllowOnly scope flag is provided")
	})

	t.Run("public environment default can be cleared for owner scope", func(t *testing.T) {
		t.Setenv(config.EnvAllowOnlyScopePublic, "true")
		t.Setenv(config.EnvAllowOnlyScopeOwner, "")
		cmd, public, owner := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{"--allowonly-scope-public=false", "--allowonly-scope-owner", "octocat"})

		require.NoError(t, cmd.Execute())
		assert.False(t, *public)
		assert.Equal(t, "octocat", *owner)
	})

	t.Run("owner environment default can be cleared for public scope", func(t *testing.T) {
		t.Setenv(config.EnvAllowOnlyScopePublic, "")
		t.Setenv(config.EnvAllowOnlyScopeOwner, "octocat")
		cmd, public, owner := setupAllowOnlyScopeCmd(t)
		cmd.SetArgs([]string{"--allowonly-scope-owner=", "--allowonly-scope-public"})

		require.NoError(t, cmd.Execute())
		assert.True(t, *public)
		assert.Empty(t, *owner)
	})
}

// TestAllowOnlyScopeFlagsRegistered verifies that the AllowOnly scope flags
// exist on the root command.
func TestAllowOnlyScopeFlagsRegistered(t *testing.T) {
	assert.NotNil(t, rootCmd.Flags().Lookup("allowonly-scope-public"), "allowonly-scope-public flag should be registered on rootCmd")
	assert.NotNil(t, rootCmd.Flags().Lookup("allowonly-scope-owner"), "allowonly-scope-owner flag should be registered on rootCmd")
}

func TestBuildAllowOnlyPolicy(t *testing.T) {
	t.Run("public scope valid", func(t *testing.T) {
		policy, err := config.BuildAllowOnlyPolicy(true, "", "", "none")
		require.NoError(t, err)
		require.NotNil(t, policy)
		require.NotNil(t, policy.AllowOnly)
		assert.Equal(t, config.IntegrityNone, policy.AllowOnly.MinIntegrity)
		assert.Equal(t, "public", policy.AllowOnly.Repos)
	})

	t.Run("owner and repo scope valid", func(t *testing.T) {
		policy, err := config.BuildAllowOnlyPolicy(false, "lpcox", "gh-aw-mcpg", "unapproved")
		require.NoError(t, err)
		require.NotNil(t, policy)
		repos, ok := policy.AllowOnly.Repos.([]string)
		require.True(t, ok)
		assert.Equal(t, []string{"lpcox/gh-aw-mcpg"}, repos)
		assert.Equal(t, config.IntegrityUnapproved, policy.AllowOnly.MinIntegrity)
	})

	t.Run("repo without owner invalid", func(t *testing.T) {
		_, err := config.BuildAllowOnlyPolicy(false, "", "repo", "unapproved")
		require.Error(t, err)
	})

	t.Run("missing min integrity invalid", func(t *testing.T) {
		_, err := config.BuildAllowOnlyPolicy(true, "", "", "")
		require.Error(t, err)
	})
}
