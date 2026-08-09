package launcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
)

// newTestPolicy builds a policy with a read-only workspace root and a
// read-write temp root, both canonicalized like production policies.
func newTestPolicy(t *testing.T) (policy MountPolicy, workspace string, tmp string) {
	t.Helper()
	base := t.TempDir()
	workspace = filepath.Join(base, "workspace")
	tmp = filepath.Join(base, "tmp")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	require.NoError(t, os.MkdirAll(tmp, 0o755))

	policy = MountPolicy{Roots: canonicalizeRoots([]MountRoot{
		{Path: workspace, Writable: false},
		{Path: tmp, Writable: true},
	})}
	return policy, workspace, tmp
}

func TestMountPolicyValidateMount(t *testing.T) {
	policy, workspace, tmp := newTestPolicy(t)

	tests := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name: "read-only workspace mount allowed",
			spec: workspace + ":/workspace:ro",
		},
		{
			name: "read-only workspace subdirectory allowed",
			spec: filepath.Join(workspace, "repo") + ":/workspace:ro",
		},
		{
			name: "read-write temp mount allowed",
			spec: tmp + ":/payloads:rw",
		},
		{
			name:    "read-write workspace mount rejected",
			spec:    workspace + ":/workspace:rw",
			wantErr: "read-write access",
		},
		{
			name:    "mount without mode defaults to read-write and is rejected",
			spec:    workspace + ":/workspace",
			wantErr: "read-write access",
		},
		{
			name:    "host path outside allowed roots rejected",
			spec:    "/etc:/host-etc:ro",
			wantErr: "outside the allowed mount roots",
		},
		{
			name:    "path traversal escape rejected",
			spec:    filepath.Join(workspace, "..", "..", "etc") + ":/host-etc:ro",
			wantErr: "outside the allowed mount roots",
		},
		{
			name:    "relative source rejected",
			spec:    "workspace:/workspace:ro",
			wantErr: "absolute path",
		},
		{
			name:    "relative destination rejected",
			spec:    workspace + ":workspace:ro",
			wantErr: "absolute path",
		},
		{
			name:    "malformed declaration rejected",
			spec:    workspace,
			wantErr: "invalid mount declaration",
		},
		{
			name:    "too many segments rejected",
			spec:    workspace + ":/workspace:ro:extra",
			wantErr: "invalid mount declaration",
		},
		{
			name:    "unsupported mount option rejected",
			spec:    workspace + ":/workspace:rslave",
			wantErr: "unsupported mount option",
		},
		{
			name:    "empty source rejected",
			spec:    ":/workspace:ro",
			wantErr: "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policy.ValidateMount(tt.spec)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMountPolicyRejectsSymlinkEscape(t *testing.T) {
	policy, workspace, _ := newTestPolicy(t)

	outside := t.TempDir()
	link := filepath.Join(workspace, "escape")
	require.NoError(t, os.Symlink(outside, link))

	err := policy.ValidateMount(link + ":/workspace:ro")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the allowed mount roots")

	// A symlink that stays inside the allowed root remains valid.
	inside := filepath.Join(workspace, "inner")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	innerLink := filepath.Join(workspace, "inner-link")
	require.NoError(t, os.Symlink(inside, innerLink))
	assert.NoError(t, policy.ValidateMount(innerLink+":/workspace:ro"))
}

func TestMountPolicyEmptyPolicyDeniesAllMounts(t *testing.T) {
	var policy MountPolicy
	err := policy.ValidateMount("/tmp:/data:ro")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the allowed mount roots")
}

func TestMountPolicyValidateContainerArgs(t *testing.T) {
	policy, workspace, tmp := newTestPolicy(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "allowed mounts pass",
			args: []string{"run", "--rm", "-i", "-v", workspace + ":/workspace:ro", "--volume", tmp + ":/payloads:rw", "image:latest"},
		},
		{
			name: "no mounts pass",
			args: []string{"run", "--rm", "-i", "-e", "NO_COLOR=1", "image:latest"},
		},
		{
			name:    "disallowed -v mount rejected",
			args:    []string{"run", "-v", "/etc:/host-etc:ro", "image:latest"},
			wantErr: "outside the allowed mount roots",
		},
		{
			name:    "disallowed --volume= mount rejected",
			args:    []string{"run", "--volume=/etc:/host-etc:ro", "image:latest"},
			wantErr: "outside the allowed mount roots",
		},
		{
			name:    "dangling -v rejected",
			args:    []string{"run", "-v"},
			wantErr: "missing mount declaration",
		},
		{
			name:    "--mount bypass rejected",
			args:    []string{"run", "--mount", "type=bind,source=/etc,target=/host-etc", "image:latest"},
			wantErr: "--mount",
		},
		{
			name:    "--volumes-from bypass rejected",
			args:    []string{"run", "--volumes-from", "other", "image:latest"},
			wantErr: "--volumes-from",
		},
		{
			name:    "--privileged bypass rejected",
			args:    []string{"run", "--privileged", "image:latest"},
			wantErr: "--privileged",
		},
		{
			name:    "--device bypass rejected",
			args:    []string{"run", "--device=/dev/sda:/dev/sda", "image:latest"},
			wantErr: "--device",
		},
		{
			name:    "Podman --rootfs bypass rejected",
			args:    []string{"run", "--rootfs=/etc", "image:latest"},
			wantErr: "--rootfs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policy.ValidateContainerArgs(tt.args)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLaunchStdioConnectionOnlyValidatesRuntimeArgs(t *testing.T) {
	cfg := newTestConfig(map[string]*config.ServerConfig{
		"entrypoint-option": {
			Type:                 "stdio",
			Containerized:        true,
			ContainerRuntimeArgs: []string{"run", "--rm"},
			Command:              "nonexistent-runtime-12345",
			Args:                 []string{"run", "--rm", "image:latest", "--privileged"},
		},
	})

	l := New(context.Background(), cfg)
	defer l.Close()

	_, err := GetOrLaunch(l, "entrypoint-option")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create connection")
	assert.NotContains(t, err.Error(), "mount policy")
}

func TestDefaultMountPolicyAllowsWorkspaceAndTemp(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv(AllowedMountRootsEnvVar, "")

	policy := DefaultMountPolicy()
	require.NotEmpty(t, policy.Roots)

	assert.NoError(t, policy.ValidateMount(workspace+":/workspace:ro"))
	assert.NoError(t, policy.ValidateMount(filepath.Join(os.TempDir(), "jq-payloads")+":/payloads:rw"))
	assert.Error(t, policy.ValidateMount("/etc:/host-etc:ro"))
	assert.Error(t, policy.ValidateMount(workspace+":/workspace:rw"))
}

func TestDefaultMountPolicyEnvOverride(t *testing.T) {
	allowed := t.TempDir()
	readonly := filepath.Join(allowed, "readonly")
	require.NoError(t, os.MkdirAll(readonly, 0o755))
	t.Setenv("GITHUB_WORKSPACE", "")
	t.Setenv(AllowedMountRootsEnvVar, allowed+":rw, relative/path, "+readonly+":ro")

	policy := DefaultMountPolicy()
	require.Len(t, policy.Roots, 2, "non-absolute roots are ignored")
	canonicalReadonly, err := filepath.EvalSymlinks(readonly)
	require.NoError(t, err)
	assert.Equal(t, canonicalReadonly, policy.Roots[0].Path, "more specific roots take precedence")

	assert.NoError(t, policy.ValidateMount(allowed+":/data:rw"))
	assert.NoError(t, policy.ValidateMount(readonly+":/data:ro"))
	assert.Error(t, policy.ValidateMount(readonly+":/data:rw"), "read-only sub-root overrides writable parent")
	assert.Error(t, policy.ValidateMount(os.TempDir()+":/data:rw"), "default roots do not apply when overridden")
}

func TestCanonicalizePathHandlesMissingLeafDirectories(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(real, 0o755))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(real, link))

	got, err := canonicalizePath(filepath.Join(link, "missing", "child"))
	require.NoError(t, err)

	resolvedReal, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(resolvedReal, "missing", "child"), got)

	_, err = canonicalizePath("relative/path")
	assert.Error(t, err)
}

// TestLaunchStdioConnectionEnforcesMountPolicy verifies that mount policy
// violations are rejected before the backend process is launched.
func TestLaunchStdioConnectionEnforcesMountPolicy(t *testing.T) {
	cfg := newTestConfig(map[string]*config.ServerConfig{
		"bad-mount": {
			Type:          "stdio",
			Containerized: true,
			Command:       "docker",
			Args:          []string{"run", "--rm", "-i", "-v", "/etc:/host-etc:ro", "image:latest"},
		},
	})

	l := New(context.Background(), cfg)
	defer l.Close()

	conn, err := GetOrLaunch(l, "bad-mount")
	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "outside the allowed mount roots")
	assert.NotContains(t, err.Error(), "failed to create connection", "launch must be blocked before process start")
}

// TestLaunchStdioConnectionSkipsPolicyForDirectCommands verifies that
// non-container stdio commands are not subject to container mount validation.
func TestLaunchStdioConnectionSkipsPolicyForDirectCommands(t *testing.T) {
	cfg := newTestConfig(map[string]*config.ServerConfig{
		"direct": {
			Type:    "stdio",
			Command: "nonexistent-command-12345",
			Args:    []string{"-v", "/etc:/host-etc:ro"},
		},
	})

	l := New(context.Background(), cfg)
	defer l.Close()

	_, err := GetOrLaunch(l, "direct")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create connection")
}

func TestCanonicalizeRootsRejectsFilesystemRoot(t *testing.T) {
	roots := canonicalizeRoots([]MountRoot{{Path: "/", Writable: true}, {Path: os.TempDir(), Writable: true}})
	require.Len(t, roots, 1)
	assert.NotEqual(t, "/", roots[0].Path)
}

func TestParseMountDeclarationRejectsConflictingModes(t *testing.T) {
	_, err := parseMountDeclaration("/srv/data:/data:ro,rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting mount options")
}
