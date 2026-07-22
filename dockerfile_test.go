package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	expectedBuilderBase = "FROM golang:1.25.11-alpine3.22@sha256:65b4400aee0927412e9ed791a11893273a49d55df24841f7599660fb80dae464 AS builder"
	expectedRuntimeBase = "FROM alpine:3.22.5@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce"
)

func TestDockerfileUsesPinnedBaseImages(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller should resolve this test file path")

	dockerfilePath := filepath.Join(filepath.Dir(thisFile), "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	require.NoError(t, err)

	dockerfile := string(content)
	assert.Contains(t, dockerfile, expectedBuilderBase)
	assert.Contains(t, dockerfile, expectedRuntimeBase)
	assert.NotContains(t, dockerfile, "FROM golang:1.25-alpine AS builder")
	assert.NotContains(t, dockerfile, "FROM alpine:latest")
}
