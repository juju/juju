// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
)

// ColumnName describes the column name when outputting the find query.
type ColumnName = string

const (
	ColumnNameName          ColumnName = "Name"
	ColumnNameBundle        ColumnName = "Bundle"
	ColumnNameVersion       ColumnName = "Version"
	ColumnNameArchitectures ColumnName = "Architectures"
	ColumnNameOS            ColumnName = "OS"
	ColumnNameSupports      ColumnName = "Supports"
	ColumnNamePublisher     ColumnName = "Publisher"
	ColumnNameSummary       ColumnName = "Summary"
)

// Column holds the column name and the index location.
type Column struct {
	Index int
	Name  ColumnName
}

// Columns represents the find columns for the find writer.
type Columns map[rune]Column

// DefaultColumns represents the default columns for the output of the find.
func DefaultColumns() Columns {
	return map[rune]Column{
		'n': {Index: 0, Name: ColumnNameName},
		'b': {Index: 1, Name: ColumnNameBundle},
		'v': {Index: 2, Name: ColumnNameVersion},
		'a': {Index: 3, Name: ColumnNameArchitectures},
		'o': {Index: 4, Name: ColumnNameOS},
		'S': {Index: 5, Name: ColumnNameSupports},
		'p': {Index: 6, Name: ColumnNamePublisher},
		's': {Index: 7, Name: ColumnNameSummary},
	}
}

// MakeColumns creates a new set of columns using the string for selecting each
// column from a base set of columns.
func MakeColumns(d Columns, s string) (Columns, error) {
	cols := make(Columns)
	for i, alias := range s {
		col, ok := d[alias]
		if !ok {
			return cols, errors.Errorf("unexpected column alias %q", alias)
		}
		cols[alias] = Column{
			Index: i,
			Name:  col.Name,
		}
	}
	return cols, nil
}

// Names returns the names of all the columns.
func (c Columns) Names() []string {
	cols := make([]Column, 0, len(c))
	for _, n := range c {
		cols = append(cols, n)
	}
	sort.Slice(cols, func(i, j int) bool {
		return cols[i].Index < cols[j].Index
	})
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return names
}

func makeFindWriter(w io.Writer, warning Log, columns Columns, in []FindResponse) Printer {
	return findWriter{
		w:        w,
		warningf: warning,
		columns:  columns,
		in:       in,
	}
}

type findWriter struct {
	warningf Log
	w        io.Writer
	columns  Columns
	in       []FindResponse
}

func (f findWriter) Print() error {
	buffer := bytes.NewBufferString("")

	tw := output.TabWriter(buffer)

	colNames := f.columns.Names()
	fmt.Fprintln(tw, strings.Join(colNames, "\t"))
	for _, result := range f.in {
		summary, err := oneLine(result.Summary, summaryColumnIndex(f.columns))
		if err != nil {
			f.warningf("%v", err)
		}

		colValues := make([]interface{}, 0, len(colNames))
		for _, name := range f.columns.Names() {
			switch name {
			case ColumnNameName:
				colValues = append(colValues, result.Name)
			case ColumnNameBundle:
				colValues = append(colValues, f.bundle(result))
			case ColumnNameVersion:
				colValues = append(colValues, result.Version)
			case ColumnNameArchitectures:
				colValues = append(colValues, strings.Join(result.Arches, ","))
			case ColumnNameOS:
				colValues = append(colValues, strings.Join(result.OS, ","))
			case ColumnNameSupports:
				colValues = append(colValues, strings.Join(result.Series, ","))
			case ColumnNamePublisher:
				colValues = append(colValues, result.Publisher)
			case ColumnNameSummary:
				colValues = append(colValues, summary)
			}
		}

		colFmt := strings.Repeat("%s\t", len(colNames))
		fmt.Fprintf(tw, colFmt[:len(colFmt)-1]+"\n", colValues...)
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

func summaryColumnIndex(columns Columns) int {
	for _, v := range columns {
		if v.Name == ColumnNameSummary {
			return v.Index
		}
	}
	return -1
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
