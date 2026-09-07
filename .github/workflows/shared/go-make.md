---
network:
  allowed:
    - go
mcp-scripts:
  go:
    description: "Execute any Go command. This tool is accessible as 'mcpscripts-go'. Provide the full command after 'go' (e.g., args: 'test ./...'). The tool will run: go <args>. Use single quotes ' for complex args to avoid shell interpretation issues."
    inputs:
      args:
        type: string
        description: "Arguments to pass to go CLI (without the 'go' prefix). Examples: 'test ./...', 'build ./cmd/gh-aw', 'mod tidy', 'fmt ./...', 'vet ./...'"
        required: true
    run: |
      echo "go $INPUT_ARGS"
      go $INPUT_ARGS

  make:
    description: "Execute any Make target. This tool is accessible as 'mcpscripts-make'. Provide the target name(s) (e.g., args: 'build'). The tool will run: make <args>. Use single quotes ' for complex args to avoid shell interpretation issues."
    inputs:
      args:
        type: string
        description: "Arguments to pass to make (target names and options). Examples: 'build', 'test-unit', 'lint', 'recompile', 'agent-finish', 'fmt build test-unit'"
        required: true
    run: |
      echo "make $INPUT_ARGS"
      make $INPUT_ARGS
---

**IMPORTANT**: Always use the `mcpscripts-go` and `mcpscripts-make` tools for Go and Make commands instead of running them directly via bash. These MCP script tools provide consistent execution and proper logging.

**Correct**:
```
Use the mcpscripts-go tool with args: "test ./..."
Use the mcpscripts-make tool with args: "build"
Use the mcpscripts-make tool with args: "lint"
Use the mcpscripts-make tool with args: "test-unit"
```

**Incorrect**:
```
Use the go MCP script tool with args: "test ./..."  ❌ (Wrong tool name - use mcpscripts-go)
Run: go test ./...  ❌ (Use mcpscripts-go instead)
Execute bash: make build  ❌ (Use mcpscripts-make instead)
```
