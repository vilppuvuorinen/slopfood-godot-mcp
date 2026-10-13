package godot

import (
	"context"
	"strconv"
)

// RunProject boots the project's main scene in a throwaway headless engine and
// quits after quitAfter main-loop iterations, forwarding extraArgs to the game.
// Headless binds the dummy display and audio drivers, so this is a boot/logic
// smoke run rather than a faithful rendering or input test. A non-zero exit is
// not an error: the game reports its own failures that way, so only a failure to
// start the process is an error.
func RunProject(ctx context.Context, executable, projectPath string, quitAfter int, extraArgs ...string) (stdout, stderr string, exitCode int, err error) {
	args := make([]string, 0, 6+len(extraArgs))
	args = append(args,
		"--headless",
		"--path", projectPath,
		"--quit-after", strconv.Itoa(quitAfter),
		"--",
	)
	args = append(args, extraArgs...)
	return runEngine(ctx, executable, args...)
}

// A non-zero exit is not an error: Godot's editor calls quit(EXIT_FAILURE) when the export fails.
func Export(ctx context.Context, executable, projectPath, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error) {
	exportFlag := "--export-release"
	if debug {
		exportFlag = "--export-debug"
	}
	return runEngine(ctx, executable,
		"--headless",
		"--path", projectPath,
		exportFlag,
		presetName,
		outputPath,
	)
}
