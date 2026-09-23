package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/tools"
)

var version = "dev"

// enableValues collects the repeatable --enable flag values verbatim.
type enableValues []string

func (e *enableValues) String() string { return strings.Join(*e, ",") }

func (e *enableValues) Set(value string) error {
	*e = append(*e, value)
	return nil
}

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
		enable      enableValues
	)
	flag.Var(&enable, "enable", "tool categories to enable (repeatable, comma-separated; default \"project\"; \"all\" selects every category)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	categories, err := tools.ResolveCategories(enable)
	if err != nil {
		return fmt.Errorf("resolve categories: %w", err)
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

	tools.Register(srv, categories...)

	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
