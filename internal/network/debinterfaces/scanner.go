// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
)

type lineScanner struct {
	filename string   // underlying source of lines[], if any
	lines    []string // all lines; never mutated
	line     string   // current line
	n        int      // current index into lines[]
	max      int      // len(lines)
}

func newScanner(filename string, src interface{}) (*lineScanner, error) {
	if filename == "" && src == nil {
		return nil, errors.New("filename and input is nil")
	}

	content, err := readSource(filename, src)

	if err != nil {
		return nil, err
	}

	lines := readLines(bytes.NewReader(content))

	return &lineScanner{
		filename: filename,
		lines:    lines,
		max:      len(lines),
	}, nil
}

// If src != nil, readSource converts src to a []byte if possible,
// otherwise it returns an error. If src == nil, readSource returns
// the result of reading the file specified by filename.
func readSource(filename string, src interface{}) ([]byte, error) {
	if src == nil {
		return os.ReadFile(filename)
	}
	switch s := src.(type) {
	case string:
		return []byte(s), nil
	}
	return nil, errors.New("invalid source type")
}

func readLines(rdr io.Reader) []string {
	lines := make([]string, 0)
	scanner := bufio.NewScanner(rdr)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

func (s *lineScanner) nextLine() bool {
	for {
		if s.n == s.max {
			return false
		}
		s.line = strings.TrimSpace(s.lines[s.n])
		s.n++
		if strings.HasPrefix(s.line, "#") || s.line == "" {
			continue
		}
		return true
	}
}
