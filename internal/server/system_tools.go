package server

import (
	"encoding/json"
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logSys = logger.New("server:system_tools")

// SysServer implements the MCPG system tools
type SysServer struct {
	serverIDs []string
}

// NewSysServer creates a new system server
func NewSysServer(serverIDs []string) *SysServer {
	logSys.Printf("Creating new SysServer with %d servers: %v", len(serverIDs), serverIDs)
	return &SysServer{
		serverIDs: serverIDs,
	}
}

// HandleRequest processes MCP requests for system tools
func (s *SysServer) HandleRequest(method string, params json.RawMessage) (interface{}, error) {
	logSys.Printf("Handling request: method=%s", method)

	switch method {
	case "tools/list":
		return s.listTools()
	case "tools/call":
		var callParams struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(params, &callParams); err != nil {
			logSys.Printf("Failed to unmarshal tool call params: %v", err)
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if callParams.Name == "" {
			logSys.Printf("Tool call missing name field")
			return nil, fmt.Errorf("invalid params: missing tool name")
		}
		logSys.Printf("Calling tool: name=%s", callParams.Name)
		return s.callTool(callParams.Name, callParams.Arguments)
	default:
		logSys.Printf("Unsupported method requested: %s", method)
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (s *SysServer) listTools() (interface{}, error) {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "sys_init",
				"description": "Initialize the MCPG system and get available MCP servers",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				"name":        "sys_list_servers",
				"description": "List all configured MCP backend servers",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}, nil
}

func (s *SysServer) callTool(name string, args map[string]interface{}) (interface{}, error) {
	logSys.Printf("Executing tool: name=%s", name)

	switch name {
	case "sys_init":
		return s.sysInit()
	case "sys_list_servers":
		return s.listServers()
	default:
		logSys.Printf("Unknown tool requested: %s", name)
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *SysServer) sysInit() (interface{}, error) {
	logSys.Printf("Initializing MCPG system with %d servers", len(s.serverIDs))
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("MCPG initialized. Available servers: %v", s.serverIDs),
			},
		},
	}, nil
}

func (s *SysServer) listServers() (interface{}, error) {
	serverList := ""
	for i, id := range s.serverIDs {
		serverList += fmt.Sprintf("%d. %s\n", i+1, id)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Configured MCP Servers:\n%s", serverList),
			},
		},
	}, nil
}
