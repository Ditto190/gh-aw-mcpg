.PHONY: all build build-gateway test test-all test-unit test-wasm test-integration test-integrity-harness capture-integrity-corpus test-copilot test-copilot-yolo test-copilot-all test-copilot-public-only test-copilot-owner-only test-copilot-repo-only test-copilot-prefix-only test-copilot-multi-only test-copilot-lockdown clean help

# Gateway Docker image name (built from repo root Dockerfile)
GATEWAY_IMAGE ?= local/gh-aw-mcpg
REPO_ROOT := $(shell cd ../.. && pwd)

# Default target
all: build test

# Build the WASM module
build:
	@echo "Building guard..."
	@cd rust-guard && ./build.sh

# Build the MCP Gateway Docker image from the repo root Dockerfile
# Depends on 'build' to ensure WASM is available for baking into the image
build-gateway: build
	@echo "Building gateway Docker image ($(GATEWAY_IMAGE))..."
	@docker build -t "$(GATEWAY_IMAGE)" "$(REPO_ROOT)"
	@echo "✓ Gateway image built: $(GATEWAY_IMAGE)"

# Run default test pipeline
test: build test-unit test-wasm
	@echo "✓ Default test pipeline complete"

# Run unit tests
test-unit: build
	@echo "Running unit tests..."
	@cd rust-guard && cargo test --lib

# Run WASM build verification
test-wasm: build
	@echo "✓ WASM build verified"

# Run integration tests (requires Docker and .env file with GitHub token)
test-integration: build build-gateway
	@echo "Running integration tests..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_integration_tests.sh

# Run Copilot with MCP Gateway (default: yolo mode)
# Prerequisites:
#   1. Install GitHub Copilot CLI: brew install --cask copilot-cli
#   2. Authenticate GitHub CLI: gh auth login
#   3. Create .env file with GITHUB_TOKEN
test-copilot: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (yolo mode)..."
	@DEBUG='*' GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh

# Copilot test modes
test-copilot-yolo: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (yolo mode - no DIFC)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh yolo

test-copilot-all: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (all mode, repos=all, min-integrity=approved)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh all

test-copilot-public-only: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (public-only mode)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh public-only

test-copilot-owner-only: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (owner-only mode, owner=lpcox)..."
	@ALLOW_OWNER=lpcox GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh owner-only

test-copilot-repo-only: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (repo-only mode, repo=lpcox/github-guard)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh repo-only

test-copilot-prefix-only: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (prefix-only mode, repos=lpcox/git-*, min-integrity=merged)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh prefix-only

test-copilot-multi-only: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (multi-only mode, repos=[lpcox/git-*,lpcox/github-guard], min-integrity=merged)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh multi-only

test-copilot-lockdown: build build-gateway
	@echo "Running Copilot with MCP Gateway Guard (lockdown mode - yolo + --lockdown-mode)..."
	@GATEWAY_IMAGE="$(GATEWAY_IMAGE)" ./scripts/run_copilot_test.sh lockdown

# Run all tests including integration and copilot (requires Docker + .env + Copilot CLI)
test-all: test test-integration test-copilot

# Run corpus-driven WASM integrity tests with replayed backend responses
test-integrity-harness: build
	@echo "Running corpus-driven WASM integrity harness tests..."
	@echo "⚠ Integrity harness tests require the Go test harness (not yet available in this checkout)"

# Refresh the integrity corpus from live open-source repositories
capture-integrity-corpus:
	@echo "Refreshing integrity corpus fixtures from GitHub API..."
	@./scripts/capture_integrity_corpus.sh

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f github-guard-rust.wasm
	@rm -rf rust-guard/target
	@echo "✓ Clean complete"

# Show help
help:
	@echo "GitHub Guard Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all                 - Build and run tests (default)"
	@echo "  build               - Build the WASM module"
	@echo "  build-gateway       - Build MCP Gateway Docker image from repo root"
	@echo "  test                - Run default pipeline (build, unit, wasm)"
	@echo "  test-all            - Run all tests (unit, wasm, integration, copilot)"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-wasm           - Verify WASM build"
	@echo "  test-integration    - Run integration tests (requires Docker + .env)"
	@echo "  test-integrity-harness - Run corpus-driven WASM integrity labeling tests"
	@echo "  capture-integrity-corpus - Refresh integrity corpus from live OSS repos"
	@echo "  test-copilot        - Run Copilot in yolo mode (default)"
	@echo "  test-copilot-yolo           - Copilot with no guard, no DIFC"
	@echo "  test-copilot-all            - Copilot with allow-only repos=all and min-integrity=approved"
	@echo "  test-copilot-public-only    - Copilot filtering private data (public repos only)"
	@echo "  test-copilot-owner-only     - Copilot filtering private data outside owner scope (lpcox)"
	@echo "  test-copilot-repo-only      - Copilot filtering data outside repo scope (lpcox/github-guard)"
	@echo "  test-copilot-prefix-only    - Copilot filtering data outside repo prefix scope (lpcox/git-*)"
	@echo "  test-copilot-multi-only     - Copilot filtering using multi-entry repo scope array"
	@echo "  clean               - Remove build artifacts"
	@echo "  help                - Show this help message"
	@echo ""
	@echo "Environment Variables:"
	@echo "  GATEWAY_IMAGE       - Docker image for gateway (default: local/gh-aw-mcpg)"
	@echo "  GITHUB_MCP_IMAGE    - Docker image for GitHub MCP server"
	@echo ""
	@echo "Copilot Prerequisites:"
	@echo "  1. Install Copilot CLI: brew install --cask copilot-cli"
	@echo "  2. Authenticate GitHub CLI: gh auth login"
	@echo "  3. Create .env with GITHUB_TOKEN"
