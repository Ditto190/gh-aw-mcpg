package server

import (
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/mcp"
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

// SysInit returns the system initialization response used by sys___init.
func (s *SysServer) SysInit() (interface{}, error) {
	logSys.Printf("Initializing MCPG system with %d servers", len(s.serverIDs))
	response := mcp.BuildMCPTextResponse(fmt.Sprintf("MCPG initialized. Available servers: %v", s.serverIDs))
	logSys.Printf("MCPG system initialized: availableServers=%v", s.serverIDs)
	return response, nil
}

// ListServers returns the configured backend server listing used by sys___list_servers.
func (s *SysServer) ListServers() (interface{}, error) {
	logSys.Printf("Listing %d configured servers", len(s.serverIDs))
	serverList := ""
	for i, id := range s.serverIDs {
		serverList += fmt.Sprintf("%d. %s\n", i+1, id)
	}

	return mcp.BuildMCPTextResponse(fmt.Sprintf("Configured MCP Servers:\n%s", serverList)), nil
}
