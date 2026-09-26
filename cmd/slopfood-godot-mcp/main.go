package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/tools"
)

var version = "dev"

// projectEnv is the environment variable naming the project to bind when
// --project is not passed.
const projectEnv = "GODOT_PROJECT"

// resolveProject determines the Godot project this server instance is bound to.
// The --project flag wins, then $GODOT_PROJECT, and the working directory is the
// last-resort fallback. The result is a cleaned absolute path; whether it is a
// valid project is left to the tools, so create_project can target a directory
// that does not exist yet.
func resolveProject(flagValue string) (string, error) {
	root := strings.TrimSpace(flagValue)
	if root == "" {
		root = strings.TrimSpace(os.Getenv(projectEnv))
	}
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		root = cwd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project path %q: %w", root, err)
	}
	return filepath.Clean(abs), nil
}

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
		project     = flag.String("project", "", "Godot project directory to bind to (default: $"+projectEnv+", then the working directory)")
		enable      enableValues
	)
	flag.Var(&enable, "enable", "tool categories to enable (repeatable, comma-separated; default \"all\"; any name replaces the default)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	root, err := resolveProject(*project)
	if err != nil {
		return err
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
		Name:    "slopfood-godot-mcp",
		Version: version,
	}, logger)
	srv.Instructions = fmt.Sprintf(
		"Drives the Godot editor headlessly to inspect and modify the Godot project at %s. "+
			"This server instance is bound to that project for its whole lifetime: every tool takes "+
			"paths relative to it, and no tool accepts or changes the project directory. Read-only "+
			"tools are safe to call freely; tools that write files may overwrite existing content.",
		root)

	logger.Info("bound project", "project", root)
	tools.Register(srv, root, categories...)

	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
