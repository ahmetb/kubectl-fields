package main

import (
	"bytes"
	"io"
	"regexp"
	"strings"
)

const alignTolerance = 60

// aligningPrinter aligns the yaml #comment text that's end of a line if they're
// within N characters of each other and in adjacent lines.
type aligningPrinter struct {
	w   io.Writer
	buf bytes.Buffer
}

func newAligningPrinter(w io.Writer) io.WriteCloser {
	return &aligningPrinter{w: w}
}

func (p *aligningPrinter) Write(b []byte) (int, error) {
	return p.buf.Write(b)
}

func (p *aligningPrinter) Close() error {
	if p.buf.Len() == 0 {
		return nil
	}
	v := p.buf.String()
	lines := strings.Split(v, "\n")
	hasTrailingNewline := strings.HasSuffix(v, "\n")
	if hasTrailingNewline {
		// remove last empty line split by trailing newline
		lines = lines[:len(lines)-1]
	}

	alignedLines := p.align(lines)
	output := strings.Join(alignedLines, "\n")
	if hasTrailingNewline {
		output += "\n"
	}

	_, err := p.w.Write([]byte(output))
	return err
}

func (p *aligningPrinter) align(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	output := make([]string, len(lines))
	copy(output, lines)

	i := 0
	for i < len(output) {
		blockEnd := p.findBlockEnd(output, i)
		if blockEnd > i { // more than 1 line in the block
			p.alignBlock(output, i, blockEnd)
		}

		if blockEnd < i {
			i++
		} else {
			i = blockEnd + 1
		}
	}
	return output
}

func (p *aligningPrinter) findBlockEnd(lines []string, start int) int {
	var i = start
	for ; i < len(lines); i++ {
		if findCommentIndex(lines[i]) == -1 {
			return i - 1
		}
	}
	return i - 1
}

func (p *aligningPrinter) alignBlock(lines []string, start, end int) {
	var commentIndices []int
	var lineIndices []int

	for i := start; i <= end; i++ {
		idx := findCommentIndex(lines[i])
		if idx != -1 {
			commentIndices = append(commentIndices, idx)
			lineIndices = append(lineIndices, i)
		}
	}

	if len(commentIndices) < 2 {
		return
	}

	min, max := minMaxOfSlice(commentIndices)
	if max-min > alignTolerance {
		return
	}

	for i, lineIdx := range lineIndices {
		idx := commentIndices[i]
		if idx < max {
			line := lines[lineIdx]
			lines[lineIdx] = line[:idx] + strings.Repeat(" ", max-idx) + line[idx:]
		}
	}
}

func minMaxOfSlice(s []int) (int, int) {
	if len(s) == 0 {
		return 0, 0
	}
	min, max := s[0], s[0]
	for _, v := range s[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

var reANSI = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*.?[0-9;]*[a-zA-Z]|\u001B[@-Z\\-_]|\u009B[0-9;]*[a-zA-Z]")

func stripANSI(b string) string {
	return reANSI.ReplaceAllString(b, "")
}

func findCommentIndex(line string) int {
	inSingleQuote := false
	inDoubleQuote := false
	lastHash := -1

	i := 0
	for i < len(line) {
		ansiMatch := reANSI.FindStringIndex(line[i:])
		if ansiMatch != nil && ansiMatch[0] == 0 {
			i += ansiMatch[1]
			continue
		}

		r := rune(line[i])
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				lastHash = i
			}
		}
		i++
	}
	return lastHash
}
