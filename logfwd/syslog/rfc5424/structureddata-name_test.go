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

type StructuredDataNameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StructuredDataNameSuite{})

func (s *StructuredDataNameSuite) TestValidateOkay(c *gc.C) {
	name := rfc5424.StructuredDataName("spam")

	err := name.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataNameSuite) TestValidateZeroValue(c *gc.C) {
	var name rfc5424.StructuredDataName

	err := name.Validate()

	c.Check(err, gc.ErrorMatches, `empty name`)
}

func (s *StructuredDataNameSuite) TestValidateUnsupportedChar(c *gc.C) {
	for i, char := range []string{"=", " ", "]", `"`} {
		c.Logf("trying #%d: %q", i, char)
		name := rfc5424.StructuredDataName("spam" + char)

		err := name.Validate()

		c.Check(err, gc.ErrorMatches, `invalid character`)
	}
}

func (s *StructuredDataNameSuite) TestValidateBadASCII(c *gc.C) {
	name := rfc5424.StructuredDataName("\x09")

	err := name.Validate()

	c.Check(err, gc.ErrorMatches, `must be printable US ASCII \(\\x09 at pos 0\)`)
}

func (s *StructuredDataNameSuite) TestValidateTooBig(c *gc.C) {
	name := rfc5424.StructuredDataName(strings.Repeat("x", 33))

	err := name.Validate()

	c.Check(err, gc.ErrorMatches, `too big \(max 32\)`)
}
