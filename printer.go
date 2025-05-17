// Copyright 2025 Ahmet Alp Balkan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
)

var pattern = regexp.MustCompile(`# <BEGIN>(.*?)<END>`)

// lineWriter ensures that each Write to the underlying writer contains the
// entire line and not just a part of it.
type lineWriter struct {
	w   io.Writer
	buf bytes.Buffer
}

func (w *lineWriter) Write(b []byte) (int, error) {
	totalWritten := 0
	for len(b) > 0 {
		// Find the newline character
		i := bytes.IndexByte(b, '\n')
		if i >= 0 {
			// Write everything up to and including the newline character
			n, err := w.buf.Write(b[:i+1])
			totalWritten += n
			if err != nil {
				return totalWritten, err
			}
			_, err = w.w.Write(w.buf.Bytes())
			if err != nil {
				return totalWritten, err
			}
			w.buf.Reset()
			b = b[i+1:]
		} else {
			// No newline character found, buffer the remaining bytes
			n, err := w.buf.Write(b)
			totalWritten += n
			if err != nil {
				return totalWritten, err
			}
			break
		}
	}
	return totalWritten, nil
}

// colorPrinter interprets sentinel values in yaml comments to colorize them
// in the output.
type colorPrinter struct {
	w io.Writer
}

// Write function scans the input for "# <BEGIN>...<END>" and colorizes the
// matched string and prints it.
func (p *colorPrinter) Write(b []byte) (int, error) {
	// Find all matches
	matches := pattern.FindAllSubmatch(b, -1)
	if matches == nil {
		// If no matches, return the length of the input and write it to the underlying writer
		n, err := p.w.Write(b)
		return n, err
	}

	// Buffer to build the final output
	var output bytes.Buffer

	// Iterate over the matches and replace the matched text with the colorized text
	lastIndex := 0
	for _, match := range matches {
		// Find the index of the match in the original byte slice
		matchIndex := pattern.FindIndex(b[lastIndex:])

		// Append the text before the match
		output.Write(b[lastIndex : lastIndex+matchIndex[0]])

		// Append the colorized matched text
		red := "\033[31m"
		clear := "\033[0m"
		colorizedText := fmt.Sprintf("%s# %s%s", red, match[1], clear)
		output.WriteString(colorizedText)

		// Update the last index
		lastIndex += matchIndex[1]
	}

	// Append any remaining text after the last match
	output.Write(b[lastIndex:])

	_, err := p.w.Write(output.Bytes())
	return len(b), err
}
