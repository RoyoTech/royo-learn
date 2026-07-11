// Package mcpserver owns the compile-validated dependency boundary for MCP.
package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// NewServer is retained as a compile-only reference until the MCP slice starts.
var NewServer = mcp.NewServer
