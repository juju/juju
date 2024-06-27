// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type machineSuite struct {
}

var _ = gc.Suite(&machineSuite{})

// TestNameValidate is testing machine names to see that they pass or don't pass
// validation.
func (*machineSuite) TestIdValidate(c *gc.C) {
	// Left empty for the moment till we define rules around the naming of
	// machines.
	tests := []struct {
		name string
		err  error
	}{}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.name)
		err := Name(test.name).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
