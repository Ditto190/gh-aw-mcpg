package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrMissingOIDCEnvVar(t *testing.T) {
	err := ErrMissingOIDCEnvVar("my-server")
	require.Error(t, err)
	require.ErrorContains(t, err, `"my-server"`)
	require.ErrorContains(t, err, "OIDC authentication")
	require.ErrorContains(t, err, "ACTIONS_ID_TOKEN_REQUEST_URL")
	require.ErrorContains(t, err, "permissions: { id-token: write }")
}
