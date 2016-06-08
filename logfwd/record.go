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

// SourceLocation holds all the information about the source code that
// caused the record to be created.
type SourceLocation struct {
	// Module is the source "module" (e.g. package) where the record
	// originated. This is optional.
	Module string

	// Filename is the path to the source file. This is required only
	// if Line is greater than 0.
	Filename string

	// Line is the line number in the source. It is optional. A negative
	// value means "not set". So does 0 if Filename is not set. If Line
	// is greater than 0 then Filename must be set.
	Line int
}

// ParseLocation converts the given info into a SourceLocation. The
// caller is responsible for validating the result.
func ParseLocation(module, location string) (SourceLocation, error) {
	loc, err := parseLocation(module, location)
	if err != nil {
		return loc, errors.Annotate(err, "failed to parse location")
	}
	return loc, nil
}

func parseLocation(module, location string) (SourceLocation, error) {
	loc := SourceLocation{
		Module:   module,
		Filename: location,
	}
	if location != "" {
		loc.Line = -1
		pos := strings.LastIndex(location, ":")
		if pos >= 0 {
			loc.Filename = location[:pos]
			lineno, err := strconv.Atoi(location[pos+1:])
			if err != nil {
				return SourceLocation{}, errors.Trace(err)
			}
			loc.Line = lineno
		}
	}
	return loc, nil
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

// Validate ensures that the location is correct.
func (loc SourceLocation) Validate() error {
	// Module may be anything, so there's nothing to check.

	// Filename may be set with no line number set, but not the other
	// way around.
	if loc.Line > 0 && loc.Filename == "" {
		return errors.NewNotValid(nil, "Line set but Filename empty")
	}

	return nil
}
