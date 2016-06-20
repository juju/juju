// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type StructuredDataParamSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StructuredDataParamSuite{})

func (s *StructuredDataParamSuite) TestStringFull(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam",
		Value: "a value",
	}

	str := param.String()

	c.Check(str, gc.Equals, `spam="a value"`)
}

func (s *StructuredDataParamSuite) TestStringZeroValue(c *gc.C) {
	var param rfc5424.StructuredDataParam

	str := param.String()

	c.Check(str, gc.Equals, `=""`)
}

func (s *StructuredDataParamSuite) TestStringEmptyName(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "",
		Value: "a value",
	}

	str := param.String()

	c.Check(str, gc.Equals, `="a value"`)
}

func (s *StructuredDataParamSuite) TestStringEmptyValue(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam",
		Value: "",
	}

	str := param.String()

	c.Check(str, gc.Equals, `spam=""`)
}

func (s *StructuredDataParamSuite) TestValidateOkay(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam",
		Value: "a value",
	}

	err := param.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataParamSuite) TestValidateZeroValue(c *gc.C) {
	var param rfc5424.StructuredDataParam

	err := param.Validate()

	c.Check(err, gc.NotNil)
}

func (s *StructuredDataParamSuite) TestValidateEmptyName(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "",
		Value: "a value",
	}

	err := param.Validate()

	c.Check(err, gc.ErrorMatches, `empty Name`)
}

func (s *StructuredDataParamSuite) TestValidateBadName(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam=",
		Value: "a value",
	}

	err := param.Validate()

	c.Check(err, gc.ErrorMatches, `bad Name "spam=": invalid character`)
}

func (s *StructuredDataParamSuite) TestValidateEmptyValue(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam",
		Value: "",
	}

	err := param.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataParamSuite) TestValidateBadValue(c *gc.C) {
	param := rfc5424.StructuredDataParam{
		Name:  "spam",
		Value: invalidUTF8,
	}

	err := param.Validate()

	c.Check(err, gc.ErrorMatches, `bad Value for "spam" (.*): invalid UTF-8`)
}

const invalidUTF8 = "\xc3\x28"
