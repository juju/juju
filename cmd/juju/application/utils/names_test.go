// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import gc "gopkg.in/check.v1"

type namesSuite struct{}

var _ = gc.Suite(&namesSuite{})

func (s *namesSuite) TestValidateApplicationName(c *gc.C) {
	tests := []struct {
		Name  string
		Error string
	}{
		{
			Name:  "application-1",
			Error: `invalid application name "application-1", unexpected number\(s\) found after hyphen`,
		},
		{
			Name:  "Application",
			Error: `invalid application name "Application", unexpected uppercase character`,
		},
		{
			Name:  "app£name",
			Error: `invalid application name "app£name", unexpected character £`,
		},
	}
	for i, test := range tests {
		c.Logf("test %d", i)
		err := ValidateApplicationName(test.Name)
		c.Assert(err, gc.ErrorMatches, test.Error)
	}
}
