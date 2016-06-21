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

type ProcIDSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ProcIDSuite{})

func (s *ProcIDSuite) TestStringOkay(c *gc.C) {
	procID := rfc5424.ProcID("spam")

	str := procID.String()

	c.Check(str, gc.Equals, "spam")
}

func (s *ProcIDSuite) TestStringZeroValue(c *gc.C) {
	var procID rfc5424.ProcID

	str := procID.String()

	c.Check(str, gc.Equals, "-")
}

func (s *ProcIDSuite) TestValidateOkay(c *gc.C) {
	procID := rfc5424.ProcID("spam")

	err := procID.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *ProcIDSuite) TestValidateZeroValue(c *gc.C) {
	var procID rfc5424.ProcID

	err := procID.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *ProcIDSuite) TestValidateReserved(c *gc.C) {
	procID := rfc5424.ProcID("-")

	err := procID.Validate()

	c.Check(err, gc.ErrorMatches, `"-" is reserved`)
}

func (s *ProcIDSuite) TestValidateBadASCII(c *gc.C) {
	procID := rfc5424.ProcID("spam\x09eggs")

	err := procID.Validate()

	c.Check(err, gc.ErrorMatches, `must be printable US ASCII \(\\x09 at pos 4\)`)
}

func (s *ProcIDSuite) TestValidateTooBig(c *gc.C) {
	procID := rfc5424.ProcID(strings.Repeat("x", 129))

	err := procID.Validate()

	c.Check(err, gc.ErrorMatches, `too big \(max 128\)`)
}
