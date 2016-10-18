// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

// Record holds all the information for a single log record.
type Record struct {
	// ID identifies the record and its position in a sequence
	// of records.
	ID int64

	// Origin describes what created the record.
	Origin Origin

	// Timestamp is when the record was created.
	Timestamp time.Time

	// Level is the basic logging level of the record.
	Level loggo.Level

	// Location describes where the record was created.
	Location SourceLocation

	// Message is the record's body. It may be empty.
	Message string
}

// Validate ensures that the record is correct.
func (rec Record) Validate() error {
	if err := rec.Origin.Validate(); err != nil {
		return errors.Annotate(err, "invalid Origin")
	}

	if rec.Timestamp.IsZero() {
		return errors.NewNotValid(nil, "empty Timestamp")
	}

	// rec.Level may be anything, so we don't check it.

	if err := rec.Location.Validate(); err != nil {
		return errors.Annotate(err, "invalid Location")
	}

	// rec.Message may be anything, so we don't check it.

	return nil
}

// SourceLocation identifies the line of source code that originated
// a log record.
type SourceLocation struct {
	// Module is the source "module" (e.g. package) where the record
	// originated. This is optional.
	Module string

	// Filename is the base name of the source file. This is required
	// only if Line is greater than 0.
	Filename string

	// Line is the line number in the source. It is optional. A negative
	// value means "not set". So does 0 if Filename is not set. If Line
	// is greater than 0 then Filename must be set.
	Line int
}

// ParseLocation converts the given info into a SourceLocation. The
// expected format is "FILENAME" or "FILENAME:LINE". If the first format
// is used then Line is set to -1. If provided, LINE must be a
// non-negative integer.
func ParseLocation(module, sourceLine string) (SourceLocation, error) {
	filename, lineNo, err := parseSourceLine(sourceLine)
	if err != nil {
		return SourceLocation{}, errors.Annotate(err, "failed to parse sourceLine")
	}
	loc := SourceLocation{
		Module:   module,
		Filename: filename,
		Line:     lineNo,
	}
	return loc, nil
}

func parseSourceLine(sourceLine string) (filename string, line int, err error) {
	filename, sep, lineNoStr := rPartition(sourceLine, ":")
	if sep == "" {
		return filename, -1, nil
	}
	if lineNoStr == "" {
		return "", -1, errors.New(`missing line number after ":"`)
	}
	lineNo, err := strconv.Atoi(lineNoStr)
	if err != nil {
		return "", -1, errors.Annotate(err, "line number must be non-negative integer")
	}
	if lineNo < 0 {
		return "", -1, errors.New("line number must be non-negative integer")
	}
	return filename, lineNo, nil
}

func rPartition(str, sep string) (remainder, used, part string) {
	pos := strings.LastIndex(str, sep)
	if pos < 0 {
		return str, "", ""
	}
	return str[:pos], sep, str[pos+1:]
}

// String returns a string representation of the location.
func (loc SourceLocation) String() string {
	if loc.Line < 0 {
		return loc.Filename
	}
	if loc.Line == 0 && loc.Filename == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", loc.Filename, loc.Line)
}

var zero SourceLocation

// Validate ensures that the location is correct.
func (loc SourceLocation) Validate() error {
	if loc == zero {
		return nil
	}

	// Module may be anything, so there's nothing to check there.

	// Filename may be set with no line number set, but not the other
	// way around.
	if loc.Line >= 0 && loc.Filename == "" {
		return errors.NewNotValid(nil, "Line set but Filename empty")
	}

	return nil
}
