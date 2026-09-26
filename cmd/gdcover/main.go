// Command gdcover instruments GDScript for line coverage and turns the hit
// traces it emits into reports. It backs the Godot half of `make e2e-cover`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gdcover:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: gdcover <instrument|lcov> [flags]")
	}
	switch args[0] {
	case "instrument":
		return runInstrument(args[1:], stdout)
	case "lcov":
		return runLCOV(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInstrument(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("instrument", flag.ContinueOnError)
	in := flags.String("in", "", "GDScript file to instrument")
	out := flags.String("out", "", "path for the instrumented script (default stdout)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *in == "" {
		return errors.New("instrument: -in is required")
	}

	src, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("read %s: %w", *in, err)
	}
	result, err := gdcover.Instrument(src)
	if err != nil {
		return fmt.Errorf("instrument %s: %w", *in, err)
	}
	if *out == "" {
		if _, err := stdout.Write(result.Source); err != nil {
			return fmt.Errorf("write instrumented script: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(*out, result.Source, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Fprintf(stdout, "instrumented %s: %d instrumentable lines\n", *in, len(result.Lines))
	return nil
}

func runLCOV(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("lcov", flag.ContinueOnError)
	source := flags.String("source", "", "original GDScript file the hits refer to")
	hitsPath := flags.String("hits", "", "hit trace file written by the instrumented script")
	out := flags.String("out", "", "LCOV tracefile to write (default stdout)")
	label := flags.String("sf", "", "path to record as SF: in the report (default -source)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *source == "" || *hitsPath == "" {
		return errors.New("lcov: -source and -hits are required")
	}

	src, err := os.ReadFile(*source)
	if err != nil {
		return fmt.Errorf("read %s: %w", *source, err)
	}
	result, err := gdcover.Instrument(src)
	if err != nil {
		return fmt.Errorf("instrument %s: %w", *source, err)
	}
	hits, err := readHits(*hitsPath)
	if err != nil {
		return err
	}

	sf := *label
	if sf == "" {
		sf = *source
	}
	cover := gdcover.Cover{Source: sf, Lines: result.Lines, Hits: hits}

	dst := stdout
	if *out != "" {
		file, err := os.Create(*out) //nolint:gosec // path is operator-provided.
		if err != nil {
			return fmt.Errorf("create %s: %w", *out, err)
		}
		defer file.Close()
		dst = file
	}
	if err := cover.WriteLCOV(dst); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Fprintf(stderr, "gdcover: %d/%d lines (%.1f%%)\n", cover.HitLines(), len(cover.Lines), cover.Percent())
	return nil
}

// readHits opens a hit trace, treating a missing file as no hits so a run that
// exercised nothing still produces an empty report.
func readHits(path string) (map[int]int, error) {
	file, err := os.Open(path) //nolint:gosec // path is operator-provided.
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]int{}, nil
		}
		return nil, fmt.Errorf("open hits %s: %w", path, err)
	}
	defer file.Close()
	hits, err := gdcover.ParseHits(file)
	if err != nil {
		return nil, fmt.Errorf("parse hits %s: %w", path, err)
	}
	return hits, nil
}
