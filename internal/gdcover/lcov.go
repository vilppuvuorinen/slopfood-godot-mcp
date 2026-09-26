package gdcover

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// Cover is the measured coverage of one instrumented script.
type Cover struct {
	Source string      // path recorded as SF: in the report
	Lines  []int       // instrumentable lines
	Hits   map[int]int // line -> execution count
}

// ParseHits reads a hit trace written by an instrumented script: original line
// numbers separated by whitespace, one per execution, in the order they ran.
func ParseHits(r io.Reader) (map[int]int, error) {
	hits := map[int]int{}
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		line, err := strconv.Atoi(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("gdcover: parse hit %q: %w", scanner.Text(), err)
		}
		hits[line]++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("gdcover: read hits: %w", err)
	}
	return hits, nil
}

// HitLines returns how many instrumentable lines ran at least once.
func (c Cover) HitLines() int {
	hit := 0
	for _, line := range c.Lines {
		if c.Hits[line] > 0 {
			hit++
		}
	}
	return hit
}

// Percent returns the covered share of instrumentable lines, 0..100. A script
// with no instrumentable lines reports 100.
func (c Cover) Percent() float64 {
	if len(c.Lines) == 0 {
		return 100
	}
	return 100 * float64(c.HitLines()) / float64(len(c.Lines))
}

// WriteLCOV writes an LCOV tracefile record. Every instrumentable line is
// reported, uncovered ones with a zero count.
func (c Cover) WriteLCOV(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "TN:")
	fmt.Fprintf(bw, "SF:%s\n", c.Source)
	lines := append([]int(nil), c.Lines...)
	sort.Ints(lines)
	for _, line := range lines {
		fmt.Fprintf(bw, "DA:%d,%d\n", line, c.Hits[line])
	}
	fmt.Fprintf(bw, "LF:%d\n", len(lines))
	fmt.Fprintf(bw, "LH:%d\n", c.HitLines())
	fmt.Fprintln(bw, "end_of_record")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("gdcover: write lcov: %w", err)
	}
	return nil
}
