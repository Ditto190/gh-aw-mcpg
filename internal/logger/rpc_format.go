// Package logger provides structured logging for the MCP Gateway.
//
// This file contains formatting and payload helper functions for RPC message logs.
//
// Text Format: Compact, single-line format optimized for grep and command-line tools
//
//	Example: "github→tools/list 1234b {...}"
//
// Markdown Format: Human-readable with syntax highlighting, suitable for documentation
//
//	Example: "**github**→`tools/list`\n\n```json\n{...}\n```"
//
// Both formats use directional arrows (→ for outbound, ← for inbound) and support
// special handling for tools/call methods by extracting and displaying tool names.
package logger

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// LogMarshaledForDebug marshals value for debug logging and dispatches to the
// provided callbacks for success or marshal failure paths.
func LogMarshaledForDebug(value interface{}, onMarshalSuccess func(string), onMarshalFailure func(error)) {
	resultJSON, err := json.Marshal(value)
	if err != nil {
		onMarshalFailure(err)
		return
	}
	onMarshalSuccess(string(resultJSON))
}

// LogMarshaledForDebugf marshals value for debug logging and dispatches to
// formatted logging functions for success or marshal failure paths.
func LogMarshaledForDebugf(
	value interface{},
	onMarshalSuccessf func(string, ...interface{}),
	successFormat string,
	onMarshalFailuref func(string, ...interface{}),
	failureFormat string,
	args ...interface{},
) {
	formatArgs := func(extra interface{}) []interface{} {
		formattedArgs := make([]interface{}, len(args)+1)
		copy(formattedArgs, args)
		formattedArgs[len(args)] = extra
		return formattedArgs
	}

	LogMarshaledForDebug(
		value,
		func(resultJSON string) {
			onMarshalSuccessf(successFormat, formatArgs(resultJSON)...)
		},
		func(marshalErr error) {
			onMarshalFailuref(failureFormat, formatArgs(marshalErr)...)
		},
	)
}

// formatRPCMessage formats an RPC message for logging
func formatRPCMessage(info *RPCMessageInfo) string {
	// Short format: server→method (or server←resp) size payload
	dir := "←"
	if info.Direction == RPCDirectionOutbound {
		dir = "→"
	}

	var sb strings.Builder

	// Server and direction
	if info.ServerID != "" {
		sb.WriteString(info.ServerID)
		sb.WriteString(dir)
		if info.Method != "" {
			sb.WriteString(info.Method)
		} else {
			sb.WriteString("resp")
		}
	}

	// Size
	if sb.Len() > 0 {
		sb.WriteByte(' ')
	}
	sb.WriteString(strconv.Itoa(info.PayloadSize))
	sb.WriteByte('b')

	// Tool name (preserved separately when the payload is redacted)
	if info.ToolName != "" {
		sb.WriteString(" tool:")
		sb.WriteString(info.ToolName)
	}

	// Error (if present)
	if info.Error != "" {
		sb.WriteString(" err:")
		sb.WriteString(info.Error)
	}

	// Payload preview (if present)
	if info.Payload != "" {
		sb.WriteByte(' ')
		sb.WriteString(info.Payload)
	}

	return sb.String()
}

// markdownToolName returns the tool name to display for a tools/call message, preferring the
// explicitly carried name (set when the payload is redacted) over parsing the payload.
func markdownToolName(info *RPCMessageInfo) string {
	if info.ToolName != "" {
		return info.ToolName
	}
	if info.Method != "tools/call" {
		return ""
	}
	return toolNameFromRequestPayload(info.Method, []byte(info.Payload))
}

// toolNameFromRequestPayload extracts params.name from a tools/call request payload.
// It returns "" for any other method or when the payload is not a parseable tools/call request.
func toolNameFromRequestPayload(method string, payload []byte) string {
	if method != "tools/call" || len(payload) == 0 {
		return ""
	}
	var data struct {
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	return data.Params.Name
}

// isEffectivelyEmpty checks if the data is effectively empty (only contains params: null)
func isEffectivelyEmpty(data map[string]interface{}) bool {
	// If empty, it's empty
	if len(data) == 0 {
		return true
	}

	// If only one field and it's "params" with null value, it's empty
	if len(data) == 1 {
		if params, ok := data["params"]; ok && params == nil {
			return true
		}
	}

	return false
}

// formatJSONWithoutFields formats JSON by removing specified fields and compacting to single line
// Returns the formatted string, a boolean indicating if the JSON was valid, and a boolean indicating if empty
func formatJSONWithoutFields(jsonStr string, fieldsToRemove []string) (string, bool, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// If not valid JSON, return as-is with false
		return jsonStr, false, false
	}

	// Remove specified fields
	for _, field := range fieldsToRemove {
		delete(data, field)
	}

	// Check if only "params": null remains (or equivalent empty state)
	isEmpty := isEffectivelyEmpty(data)

	// Re-marshal as compact single line
	formatted, err := json.Marshal(data)
	if err != nil {
		return jsonStr, false, false
	}

	return string(formatted), true, isEmpty
}

// formatRPCMessageMarkdown formats an RPC message for markdown logging
func formatRPCMessageMarkdown(info *RPCMessageInfo) string {
	// Concise format: **server**→method \n```json \n{formatted json} \n```
	var dir string
	if info.Direction == RPCDirectionOutbound {
		dir = "→"
	} else {
		dir = "←"
	}

	var message string

	// Server, direction, and method/type
	if info.ServerID != "" {
		if info.Method != "" {
			message = fmt.Sprintf("**%s**%s`%s`", info.ServerID, dir, info.Method)

			// For tools/call, extract and display the tool name. When the payload has
			// been redacted the name is carried on the info struct instead.
			if toolName := markdownToolName(info); toolName != "" {
				message += fmt.Sprintf(" `%s`", toolName)
			}
		} else {
			message = fmt.Sprintf("**%s**%s`resp`", info.ServerID, dir)
		}
	}

	// Add formatted payload in code block
	if info.Payload != "" {
		// Remove jsonrpc and method fields, then format
		formatted, isValidJSON, isEmpty := formatJSONWithoutFields(info.Payload, []string{"jsonrpc", "method"})
		if isValidJSON {
			// Don't show JSON block if it's effectively empty (only params: null)
			if !isEmpty {
				// Valid JSON: use json code block for syntax highlighting (compact single line)
				// Empty line before code block per markdown convention
				// Code fences on their own lines with compact JSON content
				message += fmt.Sprintf("\n\n```json\n%s\n```", formatted)
			}
		} else {
			// Invalid JSON: use inline backticks to avoid malformed markdown
			message += fmt.Sprintf(" `%s`", formatted)
		}
	}

	// Error (if present)
	if info.Error != "" {
		message += fmt.Sprintf(" ⚠️`%s`", info.Error)
	}

	return message
}
