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

type MsgIDSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&MsgIDSuite{})

func (s *MsgIDSuite) TestStringOkay(c *gc.C) {
	msgID := rfc5424.MsgID("spam")

	str := msgID.String()

	c.Check(str, gc.Equals, "spam")
}

func (s *MsgIDSuite) TestStringZeroValue(c *gc.C) {
	var msgID rfc5424.MsgID

	str := msgID.String()

	c.Check(str, gc.Equals, "-")
}

func (s *MsgIDSuite) TestValidateOkay(c *gc.C) {
	msgID := rfc5424.MsgID("spam")

	err := msgID.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MsgIDSuite) TestValidateZeroValue(c *gc.C) {
	var msgID rfc5424.MsgID

	err := msgID.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MsgIDSuite) TestValidateReserved(c *gc.C) {
	msgID := rfc5424.MsgID("-")

	err := msgID.Validate()

	c.Check(err, gc.ErrorMatches, `"-" is reserved`)
}

func (s *MsgIDSuite) TestValidateBadASCII(c *gc.C) {
	msgID := rfc5424.MsgID("spam\x09eggs")

	err := msgID.Validate()

	c.Check(err, gc.ErrorMatches, `must be printable US ASCII \(\\x09 at pos 4\)`)
}

func (s *MsgIDSuite) TestValidateTooBig(c *gc.C) {
	msgID := rfc5424.MsgID(strings.Repeat("x", 33))

	err := msgID.Validate()

	c.Check(err, gc.ErrorMatches, `too big \(max 32\)`)
}
