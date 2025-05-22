// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type BaseValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestBaseValidateSuite(t *testing.T) {
	tc.Run(t, &BaseValidateSuite{})
}

func (*BaseValidateSuite) TestValidate(c *tc.C) {
	expected := MustParseBaseFromString("ubuntu@20.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		expected,
	}, expected, MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, expected)
}

func (*BaseValidateSuite) TestValidateError(c *tc.C) {
	expected := MustParseBaseFromString("ubuntu@16.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		MustParseBaseFromString("ubuntu@20.04"),
	}, expected, MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("%s not supported", expected.String()))
	c.Assert(result, tc.Equals, expected)
}

func (*BaseValidateSuite) TestFallbackValidateError(c *tc.C) {
	expected := MustParseBaseFromString("ubuntu@22.04")

	result, err := ValidateBase([]Base{
		MustParseBaseFromString("ubuntu@18.04"),
		MustParseBaseFromString("ubuntu@20.04"),
	}, Base{}, expected)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("%s not supported", expected.String()))
	c.Assert(result, tc.Equals, expected)
}
