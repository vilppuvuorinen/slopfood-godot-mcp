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
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/daemonpath"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
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

// operationQueueTimeout is the queue's kill deadline for one engine operation:
// a job that outlives it is canceled, which kills its engine through
// exec.CommandContext, and the queue waits for it to stop before running
// anything else. It is a generous safety valve for a genuinely hung engine, not
// a budget; a job that still will not stop quarantines the queue and the daemon
// shuts down to be restarted.
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
	// transport is the dev-build transport override. It defaults to "auto"
	// (UDS off Windows, TCP on Windows) and is only settable when built with
	// the devtransport tag, so a release binary always resolves auto.
	transport string
	// transportSet records whether --transport was actually given, so
	// daemonChildArgs only forwards it to a spawned child that can accept it.
	transportSet bool
}

func newFlags(name string) (*flag.FlagSet, *options) {
	opts := &options{transport: "auto"}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&opts.showVersion, "version", false, "print version and exit")
	fs.StringVar(&opts.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	fs.StringVar(&opts.project, "project", "", "Godot project directory to bind to (default: $"+projectEnv+", then the working directory)")
	fs.Var(&opts.enable, "enable", "tool categories to enable (repeatable, comma-separated; default \"all\"; any name replaces the default)")
	fs.DurationVar(&opts.idleTimeout, "idle-timeout", defaultIdleTimeout, "daemon: exit after this long with no clients (0 disables)")
	fs.StringVar(&opts.logFile, "log-file", "", "daemon: log file (default: a per-project path under the runtime directory)")
	fs.BoolVar(&opts.detach, "detach", false, "daemon: re-exec as a detached child and exit")
	registerTransportFlag(fs, opts)
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
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
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
// the project, derives the project's socket path, and either dials the daemon
// already listening there or re-execs itself detached to spawn one, then pumps
// stdio to and from the socket.
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

	transport, err := parseTransport(opts.transport)
	if err != nil {
		return err
	}
	endpoint, err := newDaemonEndpoint(role.root, transport)
	if err != nil {
		return fmt.Errorf("resolve daemon endpoint: %w", err)
	}
	logPath, err := daemonpath.LogPath(role.root)
	if err != nil {
		return fmt.Errorf("resolve daemon log: %w", err)
	}

	role.logger.Info("bound project", "project", role.root, "transport", transport, "endpoint", endpoint.addr())

	// Spawning re-execs this binary as a detached daemon child. The client does
	// not create or own a daemon: the endpoint is the only rendezvous.
	spawn := func(_ context.Context) error {
		exe, exeErr := executablePath()
		if exeErr != nil {
			return fmt.Errorf("resolve executable: %w", exeErr)
		}
		if detachErr := startDetachedDaemon(exe, role.root, opts, logPath); detachErr != nil {
			return fmt.Errorf("start detached daemon: %w", detachErr)
		}
		return nil
	}
	client := mcp.NewClient(endpoint.dialFunc(), spawn, endpoint.addr(), role.logger)

	if err := client.Run(role.ctx, stdin, stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("client: %w", err)
	}
	return nil
}

// runDaemon is the daemon role: it owns the server, the queue, and the accepted
// socket connections, and serves until a signal, an expired idle timeout, or
// Close. It listens on the project's derived Unix socket, so a thin client for
// the same project reconnects to it instead of starting a second server. Its
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

	transport, err := parseTransport(opts.transport)
	if err != nil {
		return err
	}
	endpoint, err := newDaemonEndpoint(role.root, transport)
	if err != nil {
		return fmt.Errorf("resolve daemon endpoint: %w", err)
	}
	listen, err := endpoint.listenFunc()
	if err != nil {
		return fmt.Errorf("bind daemon endpoint: %w", err)
	}
	// Registered first, so it runs last: daemon.Close (below) closes the
	// listener on the normal path, and this catches an early return between
	// binding and daemon construction. TCPListener.Close is idempotent.
	defer endpoint.close()

	// The engine session outlives individual calls but not the daemon, so it is
	// closed after daemon.Close stops the queue and worker. Registering its defer
	// first makes LIFO run daemon.Close before the engine is killed, so no
	// in-flight operation loses its engine.
	script, err := godotops.OperationsScript()
	if err != nil {
		return fmt.Errorf("materialize operations script: %w", err)
	}
	session := godot.NewSessionTransport(godot.Path(), script, transport)
	defer func() {
		if cerr := session.Close(); cerr != nil {
			role.logger.Warn("engine session close", "error", cerr)
		}
	}()
	serveCtx := tools.WithEngine(role.ctx, session)

	queue := mcp.NewQueue(operationQueueDepth, operationQueueTimeout)
	daemon := mcp.NewDaemon(
		role.newServer(),
		queue,
		listen,
		endpoint.addr(),
	)
	// A quarantined queue can no longer prove that engine work is serialized,
	// so the daemon stops serving and the process exits; the host respawns it
	// on the next request. Continuing would risk two writers on the project.
	queue.SetOnStuck(func(err error) {
		role.logger.Error("operation queue stuck; shutting down so the daemon can restart", "error", err)
		daemon.Shutdown()
	})
	daemon.SetIdleTimeout(opts.idleTimeout)
	defer func() {
		if cerr := daemon.Close(); cerr != nil {
			role.logger.Warn("daemon close", "error", cerr)
		}
	}()

	role.logger.Info("daemon listening", "project", role.root, "transport", transport, "endpoint", endpoint.addr(), "log", logPath)
	if err := daemon.Serve(serveCtx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("daemon serve: %w", err)
	}
	role.logger.Info("daemon stopped", "project", role.root)
	return nil
}
