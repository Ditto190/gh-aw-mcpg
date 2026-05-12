// Package logger provides the gateway logging implementations.
//
// The package exposes three parallel global sink APIs:
//   - LogInfo / LogWarn / LogError / LogDebug for unified file/stdout logs
//   - LogInfoToMarkdown / LogWarnToMarkdown / LogErrorToMarkdown / LogDebugToMarkdown for markdown preview logs
//   - LogInfoToServer / LogWarnToServer / LogErrorToServer / LogDebugToServer
//     for per-server logs
//
// These APIs target different sinks and can be used together when a message should
// appear in multiple outputs.
package logger
