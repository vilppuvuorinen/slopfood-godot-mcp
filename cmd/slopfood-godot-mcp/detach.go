package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Spawn seams; tests replace them to observe the child command.
var (
	executablePath      = os.Executable
	startDetachedDaemon = detachDaemon
)

func daemonChildArgs(root string, opts *options, logPath string) []string {
	args := make([]string, 0, 12+2*len(opts.enable))
	args = append(args,
		daemonSubcommand,
		"--project", root,
		"--log-level", opts.logLevel,
		"--log-file", logPath,
		"--idle-timeout", opts.idleTimeout.String(),
	)
	// Forward --transport only when set, so a release child never sees an unknown flag.
	if opts.transportSet {
		args = append(args, "--transport", opts.transport)
	}
	for _, value := range opts.enable {
		args = append(args, "--enable", value)
	}
	return args
}

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
