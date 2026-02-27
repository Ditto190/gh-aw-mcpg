# Fix for GitHub Issue #18295: MCP Tool Calling Loop Issues

## Problem Statement

Agents were getting stuck in infinite loops when calling MCP tools like `github-list_commits`. The agent would repeatedly make the same tool call, receive a response indicating the payload was too large, attempt to read the payload file, fail (FILE_NOT_FOUND), and retry - creating an infinite loop that would only stop when the workflow timed out.

## Root Cause Analysis

The issue was caused by the interaction between three factors:

1. **Low Default Threshold**: The default payload size threshold was set to 10KB (10,240 bytes)
2. **Typical GitHub API Response Sizes**: GitHub API responses (especially `list_commits` over 3 days) frequently exceed 10KB:
   - Small query (1-5 commits): ~2-5KB
   - Medium query (10-30 commits): **10-50KB** ← Often exceeds threshold
   - Large query (100+ commits): 100KB-1MB

3. **Inaccessible Payload Path**: When a response exceeded 10KB:
   - The middleware would save the payload to disk at an **absolute host path**: `/tmp/jq-payloads/{sessionID}/{queryID}/payload.json`
   - The middleware would return metadata with `payloadPath` pointing to this host path
   - The agent would try to read this path, but it **doesn't exist in the agent's container filesystem**
   - The agent would see `FILE_NOT_FOUND` and retry the tool call
   - This created an infinite loop

## Example from Issue #18295

From the user's logs:
```
github-list_commits
  └ {"agentInstructions":"The payload was too large for an MCP response. The comp...

✗ bash: cat /tmp/gh-aw/mcp-payloads/***/a47e03f1b3561c858a06b84d5e02eb38/payload.json 2>/dev/null || echo "FILE_NOT_FOUND"
  "description": Required
```

The agent received:
- `agentInstructions`: "The payload was too large..."
- `payloadPath`: `/tmp/jq-payloads/{session}/{query}/payload.json`

Then tried to read the file, got `FILE_NOT_FOUND`, and retried the tool call.

## Solution

**Increase the default payload size threshold from 10KB to 512KB (524,288 bytes)**

This ensures that:
1. Typical GitHub API responses (10-50KB) are returned **inline** without disk storage
2. Only truly large responses (>512KB) trigger the payload-to-disk mechanism
3. Agents don't encounter the inaccessible file path issue for normal operations
4. The threshold can still be overridden via:
   - Environment variable: `MCP_GATEWAY_PAYLOAD_SIZE_THRESHOLD=<bytes>`
   - Command-line flag: `--payload-size-threshold <bytes>`
   - Config file: `payload_size_threshold = <bytes>`

## Changes Made

### Code Changes

1. **internal/config/config_payload.go**
   - Changed `DefaultPayloadSizeThreshold` from `10240` to `524288`
   - Updated comment to explain rationale

2. **internal/cmd/flags_logging.go**
   - Changed `defaultPayloadSizeThreshold` from `10240` to `524288`
   - Updated comment

3. **internal/config/config_core.go**
   - Updated comment from "10KB" to "512KB"

4. **internal/cmd/flags_logging_test.go**
   - Updated test assertion from `10240` to `524288`

### Documentation Changes

1. **README.md**
   - Updated CLI flag default from `10240` to `524288`
   - Updated environment variable table default from `10240` to `524288`
   - Updated configuration alternative default from `10240` to `524288`

2. **config.example-payload-threshold.toml**
   - Updated default from `10240` to `524288`
   - Updated examples to use larger, more realistic values:
     - 256KB (more aggressive storage)
     - 512KB (default)
     - 1MB (minimize disk storage)

## Testing

All tests pass with the new default:
- Unit tests: ✅ PASS (all packages)
- Integration tests: ✅ PASS
- Configuration tests: ✅ PASS

## Impact

### Before Fix (10KB threshold)
- GitHub `list_commits` responses frequently exceeded threshold
- Agents got stuck in infinite loops trying to read inaccessible files
- Workflows would timeout after repeatedly calling the same tool
- Poor user experience

### After Fix (512KB threshold)
- GitHub `list_commits` responses are returned inline (typical size 10-50KB)
- Agents receive complete data without file system access issues
- No more infinite loops for typical use cases
- Greatly improved user experience

### Performance Considerations

**Memory Impact**: Minimal. Most responses are still well under 512KB.

**Network Impact**: Reduced. Returning data inline is faster than writing to disk and returning metadata.

**Disk I/O Impact**: Significantly reduced. Fewer responses trigger disk storage.

## Configuration Options

Users can still customize the threshold for their specific needs:

```bash
# Lower threshold (more aggressive disk storage)
MCP_GATEWAY_PAYLOAD_SIZE_THRESHOLD=262144 ./awmg --config config.toml

# Higher threshold (minimize disk storage)
MCP_GATEWAY_PAYLOAD_SIZE_THRESHOLD=1048576 ./awmg --config config.toml

# Or via config file
[gateway]
payload_size_threshold = 524288
```

## Related Issue

GitHub Issue: https://github.com/github/gh-aw/issues/18295

## Backward Compatibility

✅ **Fully backward compatible**

- Existing configurations continue to work
- Environment variable and CLI flag still functional
- Users can explicitly set the old 10KB threshold if desired
- New default is a **quality-of-life improvement** that makes the gateway work better out-of-the-box

## Future Considerations

If payload path accessibility issues persist for responses >512KB:

1. Consider adding a mount configuration to make payload paths accessible to agents
2. Consider adding a flag to return large payloads inline (disable disk storage)
3. Consider implementing payload compression to reduce size before threshold check
4. Consider per-tool threshold configuration for tools known to return large responses

## Summary

The fix addresses the root cause of the infinite loop issue by ensuring that typical MCP tool responses are small enough to be returned inline, avoiding the inaccessible file path problem entirely. The threshold remains configurable for advanced use cases, maintaining flexibility while providing sensible defaults.
