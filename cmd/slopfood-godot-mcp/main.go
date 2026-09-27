package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/tools"
)

var version = "dev"

// projectEnv is the environment variable naming the project to bind when
// --project is not passed.
const projectEnv = "GODOT_PROJECT"

// daemonSubcommand selects the daemon role. The default role is the thin stdio
// client the MCP host spawns; the daemon role owns the server and the queue.
const daemonSubcommand = "daemon"

// operationQueueDepth bounds how many engine operations may wait behind the
// one the worker is running. The queue is the single serialization point for
// engine work, so a burst sheds excess calls with an overload error instead of
// piling up pending engine spawns.
const operationQueueDepth = 8

// operationQueueTimeout bounds one engine operation so a hung engine cannot
// pin the queue's one slot; the job's context is canceled and the caller sees
// a normal tool error. It is a generous safety valve, not a tight budget.
const operationQueueTimeout = 10 * time.Minute

// defaultIdleTimeout is how long a daemon may sit with no active connections
// before it exits. It is the default for -idle-timeout; zero disables idle
// exit.
const defaultIdleTimeout = 5 * time.Minute

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

// options holds the flag values both roles share. The client and daemon speak
// the same project and category flags, because they are two halves of one
// server: the client spawns the daemon with the flags it was given. The
// log-file, idle-timeout, and detach flags are daemon-role only; the client
// ignores them.
type options struct {
	showVersion bool
	logLevel    string
	project     string
	enable      enableValues
	idleTimeout time.Duration
	logFile     string
	detach      bool
}

func newFlags(name string) (*flag.FlagSet, *options) {
	opts := &options{}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&opts.showVersion, "version", false, "print version and exit")
	fs.StringVar(&opts.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	fs.StringVar(&opts.project, "project", "", "Godot project directory to bind to (default: $"+projectEnv+", then the working directory)")
	fs.Var(&opts.enable, "enable", "tool categories to enable (repeatable, comma-separated; default \"all\"; any name replaces the default)")
	fs.DurationVar(&opts.idleTimeout, "idle-timeout", defaultIdleTimeout, "daemon: exit after this long with no clients (0 disables)")
	fs.StringVar(&opts.logFile, "log-file", "", "daemon: log file (default: a per-project path under the runtime directory)")
	fs.BoolVar(&opts.detach, "detach", false, "daemon: re-exec as a detached child and exit")
	return fs, opts
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run dispatches the binary's two roles. The default is the thin stdio client
// the MCP host spawns; the daemon role is reached through the daemon
// subcommand, which the client will re-exec as a detached child in a later
// phase.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == daemonSubcommand {
		return runDaemon(args[1:], stdout, stderr)
	}
	return runClient(args, stdin, stdout, stderr)
}

// role is the state both entrypoints build the same way: the bound project, the
// resolved categories, the logger, and the signal-scoped context.
type role struct {
	root       string
	categories []tools.Category
	logger     *slog.Logger
	ctx        context.Context
	stop       context.CancelFunc
}

func setupRole(opts *options, stderr io.Writer) (*role, error) {
	root, err := resolveProject(opts.project)
	if err != nil {
		return nil, err
	}
	categories, err := tools.ResolveCategories(opts.enable)
	if err != nil {
		return nil, fmt.Errorf("resolve categories: %w", err)
	}
	logger, err := newLogger(opts.logLevel, stderr)
	if err != nil {
		return nil, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	return &role{root: root, categories: categories, logger: logger, ctx: ctx, stop: stop}, nil
}

func newLogger(logLevel string, stderr io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	return slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level})), nil
}

// newServer builds the MCP server bound to the role's project, with the
// resolved categories registered and the instructions that advertise the
// binding.
func (r *role) newServer() *mcp.Server {
	srv := mcp.NewServer(mcp.ServerInfo{
		Name:    "slopfood-godot-mcp",
		Version: version,
	}, r.logger)
	srv.Instructions = fmt.Sprintf(
		"Drives the Godot editor headlessly to inspect and modify the Godot project at %s. "+
			"This server instance is bound to that project for its whole lifetime: every tool takes "+
			"paths relative to it, and no tool accepts or changes the project directory. Read-only "+
			"tools are safe to call freely; tools that write files may overwrite existing content.",
		r.root)
	tools.Register(srv, r.root, r.categories...)
	return srv
}

// runClient is the thin client role. It is what the MCP host spawns: it binds
// the project, starts the daemon in this process (the in-process transport
// stands in for the socket until Phase 3), dials it, and pumps stdio.
func runClient(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs, opts := newFlags("slopfood-godot-mcp")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if opts.showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	role, err := setupRole(opts, stderr)
	if err != nil {
		return err
	}
	defer role.stop()

	// The in-process endpoint is the temporary rendezvous in place of the
	// socket: the daemon listens on it and the client dials it. Because spawn
	// creates the listener, the client's first dial fails and exercises the
	// same connect-or-spawn path a real transport will take.
	endpoint := mcp.NewInProcEndpoint()
	daemon := mcp.NewDaemon(
		role.newServer(),
		mcp.NewQueue(operationQueueDepth, operationQueueTimeout),
		endpoint.Listen,
		role.root,
	)
	defer func() {
		if cerr := daemon.Close(); cerr != nil {
			role.logger.Warn("daemon close", "error", cerr)
		}
	}()

	role.logger.Info("bound project", "project", role.root)

	spawn := func(spawnCtx context.Context) error {
		go func() {
			if serveErr := daemon.Serve(spawnCtx); serveErr != nil && !errors.Is(serveErr, context.Canceled) {
				role.logger.Error("daemon serve ended", "error", serveErr)
			}
		}()
		return nil
	}
	client := mcp.NewClient(endpoint.Dial, spawn, role.root, role.logger)

	if err := client.Run(role.ctx, stdin, stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("client: %w", err)
	}
	return nil
}

// runDaemon is the daemon role: it owns the server, the queue, and the accepted
// connections, and serves until a signal, an expired idle timeout, or Close.
// With the in-process transport it has no cross-process rendezvous yet, so it
// is the shape, not the deployment, until the Unix-socket backend lands. Its
// slog output goes to the per-project log file rather than stderr, because a
// detached daemon has no terminal.
func runDaemon(args []string, stdout, stderr io.Writer) error {
	fs, opts := newFlags(daemonSubcommand)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if opts.showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if opts.idleTimeout < 0 {
		return fmt.Errorf("invalid idle timeout %s: must not be negative", opts.idleTimeout)
	}
	root, err := resolveProject(opts.project)
	if err != nil {
		return err
	}
	logPath := opts.logFile
	if logPath == "" {
		logPath, err = daemonLogPath(root)
		if err != nil {
			return err
		}
	}
	if opts.detach {
		exe, exeErr := executablePath()
		if exeErr != nil {
			return fmt.Errorf("resolve executable: %w", exeErr)
		}
		return startDetachedDaemon(exe, root, opts, logPath)
	}

	logFile, err := openDaemonLog(logPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := logFile.Close(); cerr != nil {
			fmt.Fprintln(stderr, "error: close log file:", cerr)
		}
	}()

	role, err := setupRole(opts, logFile)
	if err != nil {
		return err
	}
	defer role.stop()

	endpoint := mcp.NewInProcEndpoint()
	daemon := mcp.NewDaemon(
		role.newServer(),
		mcp.NewQueue(operationQueueDepth, operationQueueTimeout),
		endpoint.Listen,
		role.root,
	)
	daemon.SetIdleTimeout(opts.idleTimeout)
	defer func() {
		if cerr := daemon.Close(); cerr != nil {
			role.logger.Warn("daemon close", "error", cerr)
		}
	}()

	role.logger.Info("daemon listening", "project", role.root, "log", logPath)
	if err := daemon.Serve(role.ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("daemon serve: %w", err)
	}
	role.logger.Info("daemon stopped", "project", role.root)
	return nil
}
