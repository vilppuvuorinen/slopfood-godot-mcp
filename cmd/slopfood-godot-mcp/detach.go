package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// executablePath and startDetachedDaemon are the spawn seams: tests replace them
// to observe the child command without launching a real process.
var (
	executablePath      = os.Executable
	startDetachedDaemon = detachDaemon
)

// daemonChildArgs returns the argv a detached daemon child should run: the
// daemon subcommand bound to the same project, with the daemon-role options
// carried over and the log path made explicit.
func daemonChildArgs(root string, opts *options, logPath string) []string {
	args := make([]string, 0, 10+2*len(opts.enable))
	args = append(args,
		daemonSubcommand,
		"--project", root,
		"--log-level", opts.logLevel,
		"--log-file", logPath,
		"--idle-timeout", opts.idleTimeout.String(),
	)
	for _, value := range opts.enable {
		args = append(args, "--enable", value)
	}
	if opts.gdDaemon {
		args = append(args, "--gd-daemon")
	}
	return args
}

// detachDaemon re-execs the binary as a detached daemon child. The child's stdio
// is the per-project log file, so it has no terminal, and its process group is
// detached by detachProcAttr on platforms that support it. The parent does not
// wait for the child: Start then Release leaves it running on its own.
func detachDaemon(exe, root string, opts *options, logPath string) error {
	logFile, err := openDaemonLog(logPath)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	//nolint:gosec // exe comes from os.Executable.
	cmd := exec.CommandContext(context.Background(), exe, daemonChildArgs(root, opts, logPath)...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachProcAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached daemon: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release detached daemon: %w", err)
	}
	return nil
}
