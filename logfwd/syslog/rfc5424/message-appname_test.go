// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type AppNameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AppNameSuite{})

func (s *AppNameSuite) TestStringOkay(c *gc.C) {
	appName := rfc5424.AppName("spam")

	str := appName.String()

	c.Check(str, gc.Equals, "spam")
}

func (s *AppNameSuite) TestStringZeroValue(c *gc.C) {
	var appName rfc5424.AppName

	str := appName.String()

	c.Check(str, gc.Equals, "-")
}

func (s *AppNameSuite) TestValidateOkay(c *gc.C) {
	appName := rfc5424.AppName("spam")

	err := appName.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *AppNameSuite) TestValidateZeroValue(c *gc.C) {
	var appName rfc5424.AppName

	err := appName.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *AppNameSuite) TestValidateReserved(c *gc.C) {
	appName := rfc5424.AppName("-")

	err := appName.Validate()

	c.Check(err, gc.ErrorMatches, `"-" is reserved`)
}

func (s *AppNameSuite) TestValidateBadASCII(c *gc.C) {
	appName := rfc5424.AppName("spam\x09eggs")

	err := appName.Validate()

	c.Check(err, gc.ErrorMatches, `must be printable US ASCII \(\\x09 at pos 4\)`)
}

func (s *AppNameSuite) TestValidateTooBig(c *gc.C) {
	appName := rfc5424.AppName(strings.Repeat("x", 49))

	err := appName.Validate()

	c.Check(err, gc.ErrorMatches, `too big \(max 48\)`)
}
