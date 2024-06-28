// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type machineSuite struct {
}

var _ = gc.Suite(&machineSuite{})

// TestNameValidate is testing good and not so good machine names to check
// that the validate method produces the correct result.
func (*machineSuite) TestNameValidate(c *gc.C) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "",
			err:  errors.NotValid,
		},
		{
			name: "40",
		},
	}

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
