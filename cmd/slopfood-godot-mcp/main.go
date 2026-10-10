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
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/client"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp/daemon"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/tools"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/watcher"
)

var version = "dev"

const projectEnv = "GODOT_PROJECT"

const daemonSubcommand = "daemon"

// operationQueueDepth bounds queued engine operations; excess calls shed with an
// overload error instead of piling up engine spawns.
const operationQueueDepth = 8

// operationQueueTimeout is the queue's kill deadline for one engine operation; a
// job that will not stop quarantines the queue and shuts the daemon down.
const operationQueueTimeout = 10 * time.Minute

const defaultIdleTimeout = 5 * time.Minute

// resolveProject picks the project: --project wins, then $GODOT_PROJECT, then
// the working directory.
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

type options struct {
	showVersion bool
	logLevel    string
	project     string
	idleTimeout time.Duration
	logFile     string
	detach      bool
	// noWatch disables the project file watcher that queues automatic rescans.
	noWatch bool
	// transport is only settable in dev builds; a release binary always resolves auto.
	transport string
	// transportSet gates forwarding --transport to a child that can accept it.
	transportSet bool
}

func newFlags(name string) (*flag.FlagSet, *options) {
	opts := &options{transport: "auto"}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&opts.showVersion, "version", false, "print version and exit")
	fs.StringVar(&opts.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	fs.StringVar(&opts.project, "project", "", "Godot project directory to bind to (default: $"+projectEnv+", then the working directory)")
	fs.DurationVar(&opts.idleTimeout, "idle-timeout", defaultIdleTimeout, "daemon: exit after this long with no clients (0 disables)")
	fs.StringVar(&opts.logFile, "log-file", "", "daemon: log file (default: a per-project path under the runtime directory)")
	fs.BoolVar(&opts.detach, "detach", false, "daemon: re-exec as a detached child and exit")
	fs.BoolVar(&opts.noWatch, "no-watch", false, "daemon: disable automatic rescans on project file changes")
	registerTransportFlag(fs, opts)
	return fs, opts
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == daemonSubcommand {
		return runDaemon(args[1:], stdout, stderr)
	}
	return runClient(args, stdin, stdout, stderr)
}

type role struct {
	root   string
	logger *slog.Logger
	ctx    context.Context
	stop   context.CancelFunc
}

func setupRole(opts *options, stderr io.Writer) (*role, error) {
	root, err := resolveProject(opts.project)
	if err != nil {
		return nil, err
	}
	logger, err := newLogger(opts.logLevel, stderr)
	if err != nil {
		return nil, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	return &role{root: root, logger: logger, ctx: ctx, stop: stop}, nil
}

func newLogger(logLevel string, stderr io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	return slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level})), nil
}

func (r *role) newServer(engine tools.Engine) *mcp.Server {
	srv := mcp.NewServer(mcp.ServerInfo{
		Name:    "slopfood-godot-mcp",
		Version: version,
	}, r.logger)
	instructions := fmt.Sprintf(
		"Drives the Godot editor headlessly to inspect and modify the Godot project at %s. "+
			"This server instance is bound to that project for its whole lifetime: paths are "+
			"relative to the project root unless a tool documents an absolute path, and no tool "+
			"accepts or changes the project directory. Read-only tools are safe to call freely; "+
			"tools that write files may overwrite existing content.",
		r.root)
	if engineVersion, err := engine.Version(r.ctx); err == nil {
		instructions += " The bound engine is Godot " + engineVersion + "."
	} else {
		instructions += " The bound engine version could not be determined from the Godot binary."
	}
	srv.Instructions = instructions
	tools.Register(srv, r.root, engine)
	return srv
}

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
	client := client.NewClient(endpoint.dialFunc(), spawn, role.logger)

	if err := client.Run(role.ctx, stdin, stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("client: %w", err)
	}
	return nil
}

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
	listener, err := endpoint.listen()
	if err != nil {
		return fmt.Errorf("bind daemon endpoint: %w", err)
	}
	// Deferred first so it runs last and still releases on an early return; Close is idempotent.
	defer endpoint.close()

	// Deferred before daemon.Close so LIFO stops the queue and worker before the engine is killed.
	script, err := godotops.OperationsScript()
	if err != nil {
		return fmt.Errorf("materialize operations script: %w", err)
	}
	// The engine path is resolved once here, at startup, and threaded into the
	// engine rather than rediscovered per request.
	enginePath := godot.Path()
	session := godot.NewSessionTransport(enginePath, script, transport)
	defer func() {
		if cerr := session.Close(); cerr != nil {
			role.logger.Warn("engine session close", "error", cerr)
		}
	}()

	engine := tools.NewGodotEngine(session, enginePath)
	server := role.newServer(engine)
	queue := mcp.NewQueue(operationQueueDepth, operationQueueTimeout)
	d := daemon.NewDaemon(
		server,
		queue,
		listener,
	)
	// Continuing with a quarantined queue risks two writers on the project.
	queue.SetOnStuck(func(err error) {
		role.logger.Error("operation queue stuck; shutting down so the daemon can restart", "error", err)
		d.Shutdown()
	})
	if err := d.SetIdleTimeout(opts.idleTimeout); err != nil {
		return fmt.Errorf("set idle timeout: %w", err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil {
			role.logger.Warn("daemon close", "error", cerr)
		}
	}()

	// Registered after daemon.Close so LIFO stops the watcher first and no
	// automatic rescan starts while the daemon is shutting down.
	if !opts.noWatch {
		watch, watchErr := watcher.New(watcher.Config{
			Root: role.root,
			Rescan: func(ctx context.Context) {
				runAutoRescan(ctx, role.logger, queue, session, role.root)
			},
		})
		if watchErr != nil {
			role.logger.Warn("file watcher unavailable; automatic rescans disabled", "error", watchErr)
		} else {
			defer watch.Close()
			role.logger.Info("watching project for changes", "project", role.root)
		}
	}

	role.logger.Info("daemon listening", "project", role.root, "transport", transport, "endpoint", endpoint.addr(), "log", logPath)
	if err := d.Serve(role.ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("daemon serve: %w", err)
	}
	role.logger.Info("daemon stopped", "project", role.root)
	return nil
}

// runAutoRescan submits an automatic rescan to the shared queue so it runs
// behind any in-flight tool call. It blocks until the queue finishes the job,
// which coalesces a stream of file changes into ordered rescans; a full queue
// or a stop drops the rescan rather than growing a backlog. The rescan is an
// ordinary request, so the engine's scan gate brackets it like any tool call.
func runAutoRescan(ctx context.Context, logger *slog.Logger, queue *mcp.Queue, session *godot.Session, root string) {
	if _, err := os.Stat(filepath.Join(root, "project.godot")); err != nil {
		return
	}
	_, err := queue.Submit(ctx, func(ctx context.Context) (any, error) {
		_, stderr, exitCode, err := session.Run(ctx, root, "rescan_project", nil)
		if err != nil {
			return nil, fmt.Errorf("auto rescan: %w", err)
		}
		if exitCode != 0 {
			return nil, fmt.Errorf("rescan failed: %s", strings.TrimSpace(stderr))
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, mcp.ErrQueueClosed) {
		logger.Warn("automatic rescan failed", "error", err)
	}
}
