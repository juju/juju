// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&confSuite{})

type confSuite struct {
	testing.IsolationSuite

	conf *initsystems.Conf
}

func (s *confSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.conf = &initsystems.Conf{
		Desc: "an important service",
		Cmd:  "<do something>",
	}
}

func (s *confSuite) TestRepairNoop(c *gc.C) {
	origErr := errors.New("<unknown>")
	err := s.conf.Repair(origErr)

	c.Check(err, gc.Equals, origErr)
}

func (s *confSuite) TestRepairNotSupported(c *gc.C) {
	origErr := errors.NotSupportedf("<unknown>")
	err := s.conf.Repair(origErr)

	c.Check(err, gc.Equals, origErr)
}

func (s *confSuite) TestRepairUnknownField(c *gc.C) {
	errs := []error{
		initsystems.NewUnsupportedField("<unknown>"),
		initsystems.NewUnsupportedItem("<unknown>", "<a key>"),
	}

	for _, origErr := range errs {
		c.Logf("checking %v", origErr)
		err := s.conf.Repair(origErr)

		c.Check(err, gc.ErrorMatches, `reported unknown field "<unknown>" as unsupported: .*`)
		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairRequiredFields(c *gc.C) {
	for _, field := range []string{"Desc", "Cmd"} {
		c.Logf("checking %q", field)
		origErr := initsystems.NewUnsupportedField(field)
		err := s.conf.Repair(origErr)

		c.Check(err, gc.ErrorMatches, `reported required field "`+field+`" as unsupported: .*`)
		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairOut(c *gc.C) {
	s.conf.Out = "<some command>"

	origErr := initsystems.NewUnsupportedField("Out")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Out, gc.Equals, "")
}

func (s *confSuite) TestRepairNotMap(c *gc.C) {
	for _, field := range []string{"Desc", "Cmd", "Out"} {
		c.Logf("checking %q", field)
		origErr := initsystems.NewUnsupportedItem(field, "spam")
		err := s.conf.Repair(origErr)

		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairEnv(c *gc.C) {
	s.conf.Env = map[string]string{
		"x": "y",
		"w": "z",
	}

	origErr := initsystems.NewUnsupportedItem("Env", "w")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Env, jc.DeepEquals, map[string]string{"x": "y"})
}

func (s *confSuite) TestRepairEnvField(c *gc.C) {
	s.conf.Env = map[string]string{"x": "y"}

	origErr := initsystems.NewUnsupportedField("Env")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Env, gc.IsNil)
}

func (s *confSuite) TestRepairLimit(c *gc.C) {
	s.conf.Limit = map[string]string{
		"x": "y",
		"w": "z",
	}

	origErr := initsystems.NewUnsupportedItem("Limit", "w")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Limit, jc.DeepEquals, map[string]string{"x": "y"})
}

func (s *confSuite) TestRepairLimitField(c *gc.C) {
	s.conf.Limit = map[string]string{"x": "y"}

	origErr := initsystems.NewUnsupportedField("Limit")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Limit, gc.IsNil)
}

func (s *confSuite) TestValidateValid(c *gc.C) {
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.ErrorIsNil)
}

func (s *confSuite) TestValidateMissingName(c *gc.C) {
	err := s.conf.Validate("")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing name.*`)
}

func (s *confSuite) TestValidateMissingDesc(c *gc.C) {
	s.conf.Desc = ""
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Desc.*`)
}

func (s *confSuite) TestValidateMissingCmd(c *gc.C) {
	s.conf.Cmd = ""
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Cmd.*`)
}

func (s *confSuite) TestEqualsSame(c *gc.C) {
	equal := s.conf.Equals(*s.conf)

	c.Check(equal, jc.IsTrue)
}

func (s *confSuite) TestEqualsDifferent(c *gc.C) {
	other := *s.conf
	other.Cmd = "<do something else>"
	equal := s.conf.Equals(other)

	c.Check(equal, jc.IsFalse)
}
