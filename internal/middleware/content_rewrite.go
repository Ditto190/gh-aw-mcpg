package middleware

import (
	"encoding/json"

	"github.com/github/gh-aw-mcpg/internal/logger"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func rewriteFilteredTextPayload(result *sdk.CallToolResult, data any, filteredText string) (*sdk.CallToolResult, any) {
	logMiddleware.Printf("rewriteFilteredTextPayload: rewriting first text content item, filteredLen=%d", len(filteredText))
	rewrittenContent := make([]sdk.Content, 0, len(result.Content))
	rewrittenContent = append(rewrittenContent, &sdk.TextContent{Text: filteredText})
	if len(result.Content) > 1 {
		rewrittenContent = append(rewrittenContent, result.Content[1:]...)
	}

	rewrittenResult := &sdk.CallToolResult{
		Content: rewrittenContent,
		IsError: result.IsError,
		Meta:    result.Meta,
	}

	if rewrittenData, ok := rewriteEnvelopeTextPayload(data, filteredText); ok {
		return rewrittenResult, rewrittenData
	}

	var filteredPayload any
	if err := json.Unmarshal([]byte(filteredText), &filteredPayload); err == nil {
		return rewrittenResult, filteredPayload
	}
	logger.LogWarn("payload", "Failed to unmarshal filtered text payload for rewritten data, returning original backing data")

	return rewrittenResult, data
}

func rewriteEnvelopeTextPayload(data any, filteredText string) (any, bool) {
	switch v := data.(type) {
	case map[string]any:
		contentValue, ok := v["content"]
		if !ok {
			return nil, false
		}
		rewrittenMap := make(map[string]any, len(v))
		for key, value := range v {
			rewrittenMap[key] = value
		}

		rewrittenContent, ok := rewriteFirstContentItem(contentValue, filteredText)
		if !ok {
			return nil, false
		}
		rewrittenMap["content"] = rewrittenContent
		return rewrittenMap, true
	default:
		return nil, false
	}
}

func rewriteFirstContentItem(contentValue any, filteredText string) (any, bool) {
	switch content := contentValue.(type) {
	case []map[string]any:
		if len(content) == 0 {
			return nil, false
		}
		rewrittenContent := append([]map[string]any(nil), content...)
		rewrittenContent[0] = rewriteContentItemText(rewrittenContent[0], filteredText)
		return rewrittenContent, true
	case []any:
		if len(content) == 0 {
			return nil, false
		}
		rewrittenContent := append([]any(nil), content...)
		firstItem, ok := rewrittenContent[0].(map[string]any)
		if !ok {
			return nil, false
		}
		rewrittenContent[0] = rewriteContentItemText(firstItem, filteredText)
		return rewrittenContent, true
	default:
		return nil, false
	}
}

func rewriteContentItemText(contentItem map[string]any, filteredText string) map[string]any {
	rewrittenItem := make(map[string]any, len(contentItem))
	for key, value := range contentItem {
		rewrittenItem[key] = value
	}
	rewrittenItem["text"] = filteredText
	return rewrittenItem
}
