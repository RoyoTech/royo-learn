package mcpserver

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerMiddleware adds logging and error-recovery middleware to the MCP server.
func registerMiddleware(server *mcp.Server, maxRequestBytes int64) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Panic recovery middleware (receiving side).
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					logger.Error("mcp_panic_recovered",
						"panic", r,
						"method", method,
						"stack", stack,
					)
				}
			}()
			return next(ctx, method, req)
		}
	})

	// Logging middleware (sending side).
	server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil {
				logger.Error("mcp_method_error",
					"method", method,
					"error", err.Error(),
				)
			}
			return result, err
		}
	})
}
