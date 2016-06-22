// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type StructuredDataParamValueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StructuredDataParamValueSuite{})

func (s *StructuredDataParamValueSuite) TestStringOkay(c *gc.C) {
	val := rfc5424.StructuredDataParamValue("a value")

	str := val.String()

	c.Check(str, gc.Equals, "a value")
}

func (s *StructuredDataParamValueSuite) TestStringZeroValue(c *gc.C) {
	var val rfc5424.StructuredDataParamValue

	str := val.String()

	c.Check(str, gc.Equals, "")
}

func (s *StructuredDataParamValueSuite) TestStringEscaped(c *gc.C) {
	for i, char := range []string{`"`, `\`, "]"} {
		c.Logf("trying #%d: %q", i, char)
		val := rfc5424.StructuredDataParamValue("a value" + char)

		str := val.String()

		c.Check(str, gc.Equals, `a value\`+char)
	}
}

func (s *StructuredDataParamValueSuite) TestValidateOkay(c *gc.C) {
	val := rfc5424.StructuredDataParamValue("a value")

	err := val.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataParamValueSuite) TestValidateZeroValue(c *gc.C) {
	var val rfc5424.StructuredDataParamValue

	err := val.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *StructuredDataParamValueSuite) TestValidateBadUTF8(c *gc.C) {
	val := rfc5424.StructuredDataParamValue(invalidUTF8)

	err := val.Validate()

	c.Check(err, gc.ErrorMatches, `invalid UTF-8`)
}
