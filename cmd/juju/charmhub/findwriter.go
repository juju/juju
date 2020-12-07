// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
)

func makeFindWriter(w io.Writer, warning Log, in []FindResponse) Printer {
	writer := findWriter{
		w:        w,
		warningf: warning,
		in:       in,
	}
	return writer
}

type findWriter struct {
	warningf Log
	w        io.Writer
	in       []FindResponse
}

func (f findWriter) Print() error {
	buffer := bytes.NewBufferString("")

	tw := output.TabWriter(buffer)

	fmt.Fprintf(tw, "Name\tBundle\tVersion\tArchitectures\tSupports\tPublisher\tSummary\n")
	for _, result := range f.in {
		summary, err := oneLine(result.Summary, 6)
		if err != nil {
			f.warningf("%v", err)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			result.Name,
			f.bundle(result),
			result.Version,
			strings.Join(result.Arches, ","),
			strings.Join(result.Series, ","),
			result.Publisher,
			summary,
		)
	}

	if err := tw.Flush(); err != nil {
		f.warningf("%v", errors.Annotate(err, "could not flush data to buffer"))
	}

	_, err := fmt.Fprintf(f.w, "%s\n", buffer.String())
	return err
}

func (f findWriter) bundle(result FindResponse) string {
	if result.Type == "bundle" {
		return "Y"
	}
	return "-"
}

func oneLine(line string, inset int) (string, error) {
	// To ensure we don't break the tabular output, we select the first line
	// from the summary and output the first one.
	scanner := bufio.NewScanner(bytes.NewBufferString(strings.TrimSpace(line)))
	scanner.Split(bufio.ScanLines)

	var summary string
	for scanner.Scan() {
		summary = scanner.Text()
		break
	}
	if err := scanner.Err(); err != nil {
		return summary, errors.Annotate(err, "could not gather summary")
	}

	return wordWrapLine(summary, inset, 40), nil
}

// wordWrapLine attempts to wrap lines to a limit. The insert allows the offset
// of the line to a given tab to correctly display the new summary lines.
func wordWrapLine(line string, inset, limit int) string {
	var (
		current int
		lines   = [][]rune{{}}
	)

	for _, char := range line {
		// If it's a space and we're over the limit then we can assume we're
		// a word break, if so, let's wrap it.
		if len(lines[current])+1 > limit {
			if char == '-' {
				// We want the hyphen at the tail of the line, before the wrap.
				lines[current] = append(lines[current], char)
				current++
				lines = append(lines, []rune{})
				continue
			}
			if unicode.IsSpace(char) {
				current++
				lines = append(lines, []rune{})
				continue
			}
		}
		lines[current] = append(lines[current], char)
	}

	var res string
	for i, line := range lines {
		res += string(line)
		if i < len(lines)-1 {
			res += "\n" + strings.Repeat("\t", inset)
		}
	}
	return res
}
