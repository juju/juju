// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/logfwd"
)

type OriginTypeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OriginTypeSuite{})

func (s *OriginTypeSuite) TestZeroValue(c *gc.C) {
	var ot logfwd.OriginType

	c.Check(ot, gc.Equals, logfwd.OriginTypeUnknown)
}

func (s *OriginTypeSuite) TestParseOriginTypeValid(c *gc.C) {
	tests := map[string]logfwd.OriginType{
		"unknown": logfwd.OriginTypeUnknown,
		"user":    logfwd.OriginTypeUser,
		"machine": logfwd.OriginTypeMachine,
		"unit":    logfwd.OriginTypeUnit,
	}
	for str, expected := range tests {
		c.Logf("trying %q", str)

		ot, err := logfwd.ParseOriginType(str)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(ot, gc.Equals, expected)
	}
}

func (s *OriginTypeSuite) TestParseOriginTypeEmpty(c *gc.C) {
	_, err := logfwd.ParseOriginType("")

	c.Check(err, gc.ErrorMatches, `unrecognized origin type ""`)
}

func (s *OriginTypeSuite) TestParseOriginTypeInvalid(c *gc.C) {
	_, err := logfwd.ParseOriginType("spam")

	c.Check(err, gc.ErrorMatches, `unrecognized origin type "spam"`)
}

func (s *OriginTypeSuite) TestString(c *gc.C) {
	tests := map[logfwd.OriginType]string{
		logfwd.OriginTypeUnknown: "unknown",
		logfwd.OriginTypeUser:    "user",
		logfwd.OriginTypeMachine: "machine",
		logfwd.OriginTypeUnit:    "unit",
	}
	for ot, expected := range tests {
		c.Logf("trying %q", ot)

		str := ot.String()

		c.Check(str, gc.Equals, expected)
	}
}

func (s *OriginTypeSuite) TestValidateValid(c *gc.C) {
	tests := []logfwd.OriginType{
		logfwd.OriginTypeUnknown,
		logfwd.OriginTypeUser,
		logfwd.OriginTypeMachine,
		logfwd.OriginTypeUnit,
	}
	for _, ot := range tests {
		c.Logf("trying %q", ot)

		err := ot.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *OriginTypeSuite) TestValidateZero(c *gc.C) {
	var ot logfwd.OriginType

	err := ot.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *OriginTypeSuite) TestValidateInvalid(c *gc.C) {
	ot := logfwd.OriginType(999)

	err := ot.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `unsupported origin type`)
}

func (s *OriginTypeSuite) TestValidateNameValid(c *gc.C) {
	tests := map[logfwd.OriginType]string{
		logfwd.OriginTypeUnknown: "",
		logfwd.OriginTypeUser:    "a-user",
		logfwd.OriginTypeMachine: "99",
		logfwd.OriginTypeUnit:    "svc-a/0",
	}
	for ot, name := range tests {
		c.Logf("trying %q + %q", ot, name)

		err := ot.ValidateName(name)

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *OriginTypeSuite) TestValidateNameInvalid(c *gc.C) {
	tests := []struct {
		ot   logfwd.OriginType
		name string
		err  string
	}{{
		ot:   logfwd.OriginTypeUnknown,
		name: "...",
		err:  `origin name must not be set if type is unknown`,
	}, {
		ot:   logfwd.OriginTypeUser,
		name: "...",
		err:  `bad user name`,
	}, {
		ot:   logfwd.OriginTypeMachine,
		name: "...",
		err:  `bad machine name`,
	}, {
		ot:   logfwd.OriginTypeUnit,
		name: "...",
		err:  `bad unit name`,
	}}
	for _, test := range tests {
		c.Logf("trying %q + %q", test.ot, test.name)

		err := test.ot.ValidateName(test.name)

		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, test.err)
	}
}
