package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/tools"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		logLevel    = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", *logLevel, err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := mcp.NewServer(mcp.ServerInfo{
		Name:    "go-dot-mcp",
		Version: version,
	}, logger)

	srv.RegisterTool(mcp.Tool{
		Name:        "hello",
		Description: "Say hello. Returns a friendly greeting, optionally addressed to a name.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": `Who to greet. Defaults to "World".`,
				},
			},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			name := "World"
			if len(args) > 0 && string(args) != "null" {
				var in struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if in.Name != "" {
					name = in.Name
				}
			}
			return map[string]any{"message": "Hello, " + name + "!"}, nil
		},
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "echo",
		Description: "Echo back the provided message.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"message"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			return in.Message, nil
		},
	})

	tools.Register(srv)

	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
