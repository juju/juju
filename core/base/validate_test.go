// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type BaseValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BaseValidateSuite{})

func (*BaseValidateSuite) TestValidate(c *gc.C) {
	expected := MustParseBaseFromString("ubuntu@20.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		expected,
	}, expected, MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, expected)
}

func (*BaseValidateSuite) TestValidateError(c *gc.C) {
	expected := MustParseBaseFromString("ubuntu@16.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		MustParseBaseFromString("ubuntu@20.04"),
	}, expected, MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("%s not supported", expected.String()))
	c.Assert(result, gc.Equals, expected)
}

func (*BaseValidateSuite) TestFallbackValidateError(c *gc.C) {
	expected := MustParseBaseFromString("ubuntu@22.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		MustParseBaseFromString("ubuntu@20.04"),
	}, Base{}, expected)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("%s not supported", expected.String()))
	c.Assert(result, gc.Equals, expected)
}
