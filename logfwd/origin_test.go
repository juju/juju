// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd"
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

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `unsupported origin type`)
}

func (s *OriginTypeSuite) TestValidateNameValid(c *gc.C) {
	tests := map[logfwd.OriginType]string{
		logfwd.OriginTypeUnknown: "",
		logfwd.OriginTypeUser:    "a-user",
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
	}}
	for _, test := range tests {
		c.Logf("trying %q + %q", test.ot, test.name)

		err := test.ot.ValidateName(test.name)

		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, test.err)
	}
}

type OriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OriginSuite{})

func (s *OriginSuite) TestPrivateEnterpriseCode(c *gc.C) {
	var origin logfwd.Origin

	id := origin.PrivateEnterpriseCode()

	c.Check(id, gc.Equals, "28978")
}

func (s *OriginSuite) TestSoftwareName(c *gc.C) {
	var origin logfwd.Origin

	swName := origin.SoftwareName()

	c.Check(swName, gc.Equals, "jujud")
}

func (s *OriginSuite) TestValidateValid(c *gc.C) {
	origin := validOrigin

	err := origin.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateEmpty(c *gc.C) {
	var origin logfwd.Origin

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *OriginSuite) TestValidateEmptyControllerUUID(c *gc.C) {
	origin := validOrigin
	origin.ControllerUUID = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty ControllerUUID`)
}

func (s *OriginSuite) TestValidateBadControllerUUID(c *gc.C) {
	origin := validOrigin
	origin.ControllerUUID = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `ControllerUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateEmptyModelUUID(c *gc.C) {
	origin := validOrigin
	origin.ModelUUID = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty ModelUUID`)
}

func (s *OriginSuite) TestValidateBadModelUUID(c *gc.C) {
	origin := validOrigin
	origin.ModelUUID = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `ModelUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateBadOriginType(c *gc.C) {
	origin := validOrigin
	origin.Type = logfwd.OriginType(999)

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Type: unsupported origin type`)
}

func (s *OriginSuite) TestValidateEmptyName(c *gc.C) {
	origin := validOrigin
	origin.Name = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty Name`)
}

func (s *OriginSuite) TestValidateBadName(c *gc.C) {
	origin := validOrigin
	origin.Name = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Name "...": bad user name`)
}

func (s *OriginSuite) TestValidateEmptyVersion(c *gc.C) {
	origin := validOrigin
	origin.JujuVersion = version.Zero

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty JujuVersion`)
}

var validOrigin = logfwd.Origin{
	ControllerUUID: "9f484882-2f18-4fd2-967d-db9663db7bea",
	ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
	Type:           logfwd.OriginTypeUser,
	Name:           "a-user",
	JujuVersion:    version.MustParse("2.0.1"),
}
