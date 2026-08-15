package mcptest

import (
	"context"
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/mcp"
	"github.com/github/gh-aw-mcpg/internal/sanitize"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var logValidator = logger.New("testutil:validator")

const validatorPaginationMaxPages = 1000

// ValidatorClient is a client for validating MCP servers
type ValidatorClient struct {
	client  *sdk.Client
	session *sdk.ClientSession
	ctx     context.Context
}

// NewValidatorClient creates a new validator client connected to the given transport
func NewValidatorClient(ctx context.Context, transport sdk.Transport) (*ValidatorClient, error) {
	logValidator.Print("Creating validator client and connecting to transport")
	client := sdk.NewClient(&sdk.Implementation{
		Name:    "mcp-validator",
		Version: "1.0.0",
	}, &sdk.ClientOptions{
		Logger: logger.NewSlogLoggerWithHandler(logValidator),
	})

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		logValidator.Printf("Failed to connect validator client: %v", err)
		return nil, fmt.Errorf("connect to server: %w", err)
	}

	logValidator.Print("Validator client connected successfully")
	return &ValidatorClient{
		client:  client,
		session: session,
		ctx:     ctx,
	}, nil
}

// ListTools retrieves the list of tools from the connected MCP server, including all paginated results.
func (v *ValidatorClient) ListTools() ([]*sdk.Tool, error) {
	logValidator.Print("Listing tools from validator client")
	tools, err := mcp.PaginateAll(validatorPaginationMaxPages, func(cursor string) ([]*sdk.Tool, string, error) {
		result, err := v.session.ListTools(v.ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Tools, result.NextCursor, nil
	})
	if err != nil {
		logValidator.Printf("ListTools failed: %v", err)
		return nil, fmt.Errorf("list tools: %w", err)
	}
	logValidator.Printf("ListTools succeeded: count=%d", len(tools))
	return tools, nil
}

// ListResources retrieves the list of resources from the connected MCP server, including all paginated results.
func (v *ValidatorClient) ListResources() ([]*sdk.Resource, error) {
	logValidator.Print("Listing resources from validator client")
	resources, err := mcp.PaginateAll(validatorPaginationMaxPages, func(cursor string) ([]*sdk.Resource, string, error) {
		result, err := v.session.ListResources(v.ctx, &sdk.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Resources, result.NextCursor, nil
	})
	if err != nil {
		logValidator.Printf("ListResources failed: %v", err)
		return nil, fmt.Errorf("list resources: %w", err)
	}
	logValidator.Printf("ListResources succeeded: count=%d", len(resources))
	return resources, nil
}

// CallTool calls a tool on the MCP server
func (v *ValidatorClient) CallTool(name string, arguments map[string]interface{}) (*sdk.CallToolResult, error) {
	logValidator.Printf("Calling tool: name=%s, argumentCount=%d", name, len(arguments))
	result, err := v.session.CallTool(v.ctx, &sdk.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		logValidator.Printf("CallTool failed: name=%s, err=%v", name, err)
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}
	return result, nil
}

// ReadResource reads a resource from the MCP server
func (v *ValidatorClient) ReadResource(uri string) (*sdk.ReadResourceResult, error) {
	logValidator.Printf("Reading resource: uri=%s", sanitize.RedactURL(uri))
	result, err := v.session.ReadResource(v.ctx, &sdk.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		logValidator.Printf("ReadResource failed: uri=%s, err=%v", sanitize.RedactURL(uri), err)
		return nil, fmt.Errorf("read resource %s: %w", uri, err)
	}
	return result, nil
}

// GetServerInfo returns the server information from the initialize handshake
func (v *ValidatorClient) GetServerInfo() *sdk.Implementation {
	initResult := v.session.InitializeResult()
	if initResult != nil {
		return initResult.ServerInfo
	}
	return nil
}

// Close closes the validator client connection
func (v *ValidatorClient) Close() error {
	if v.session != nil {
		return v.session.Close()
	}
	return nil
}
