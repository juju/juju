// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/logfwd"
)

type RecordSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RecordSuite{})

func (s *RecordSuite) TestValidateValid(c *gc.C) {
	rec := validRecord

	err := rec.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *RecordSuite) TestValidateZero(c *gc.C) {
	var rec logfwd.Record

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *RecordSuite) TestValidateBadOrigin(c *gc.C) {
	rec := validRecord
	rec.Origin.Name = "..."

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Origin: invalid Name "...": bad user name`)
}

func (s *RecordSuite) TestValidateEmptyTimestamp(c *gc.C) {
	rec := validRecord
	rec.Timestamp = time.Time{}

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty Timestamp`)
}

func (s *RecordSuite) TestValidateBadLocation(c *gc.C) {
	rec := validRecord
	rec.Location.Filename = ""

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Location: Line set but Filename empty`)
}

type LocationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LocationSuite{})

func (s *LocationSuite) TestParseLocationTooLegitToQuit(c *gc.C) {
	expected := validLocation

	loc, err := logfwd.ParseLocation(expected.Module, expected.String())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(loc, jc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationIsValid(c *gc.C) {
	expected := validLocation
	loc, err := logfwd.ParseLocation(expected.Module, expected.String())
	c.Assert(err, jc.ErrorIsNil)

	err = loc.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *LocationSuite) TestParseLocationMissingFilename(c *gc.C) {
	expected := validLocation
	expected.Filename = ""

	loc, err := logfwd.ParseLocation(expected.Module, ":42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(loc, jc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationBogusFilename(c *gc.C) {
	expected := validLocation
	expected.Filename = "..."

	loc, err := logfwd.ParseLocation(expected.Module, "...:42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(loc, jc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationFilenameOnly(c *gc.C) {
	expected := validLocation
	expected.Line = -1

	loc, err := logfwd.ParseLocation(expected.Module, expected.Filename)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(loc, jc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationMissingLine(c *gc.C) {
	_, err := logfwd.ParseLocation(validLocation.Module, "spam.go:")

	c.Check(err, gc.ErrorMatches, `failed to parse sourceLine: missing line number after ":"`)
}

func (s *LocationSuite) TestParseLocationBogusLine(c *gc.C) {
	_, err := logfwd.ParseLocation(validLocation.Module, "spam.go:xxx")

	c.Check(err, gc.ErrorMatches, `failed to parse sourceLine: line number must be non-negative integer: strconv.(ParseInt|Atoi): parsing "xxx": invalid syntax`)
}

func (s *LocationSuite) TestValidateValid(c *gc.C) {
	loc := validLocation

	err := loc.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *LocationSuite) TestValidateEmpty(c *gc.C) {
	var loc logfwd.SourceLocation

	err := loc.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *LocationSuite) TestValidateBadLine(c *gc.C) {
	loc := validLocation
	loc.Filename = ""

	err := loc.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `Line set but Filename empty`)
}

var validLocation = logfwd.SourceLocation{
	Module:   "spam",
	Filename: "eggs.go",
	Line:     42,
}

var validRecord = logfwd.Record{
	Origin:    validOrigin,
	Timestamp: time.Now(),
	Level:     loggo.ERROR,
	Location:  validLocation,
	Message:   "uh-oh",
}
