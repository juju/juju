// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type machineSuite struct {
	testhelpers.IsolationSuite
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

// TestNameValidate is testing good and not so good machine names to check
// that the validate method produces the correct result.
func (*machineSuite) TestNameValidate(c *tc.C) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "",
			err:  coreerrors.NotValid,
		},
		{
			name: "40",
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.name)
		err := Name(test.name).Validate()

		if test.err == nil {
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}

func (*machineSuite) TestNamedChild(c *tc.C) {
	tests := []struct {
		name      Name
		scope     string
		childName string
		output    Name
		err       error
	}{
		{
			name:      "foo",
			scope:     "bar",
			childName: "baz",
			output:    Name("foo/bar/baz"),
		},
		{
			name:      "foo",
			scope:     "",
			childName: "baz",
			err:       coreerrors.NotValid,
		},
		{
			name:      "foo",
			scope:     "bar",
			childName: "",
			err:       coreerrors.NotValid,
		},
		{
			name:      "foo",
			scope:     "",
			childName: "",
			err:       coreerrors.NotValid,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.output)

		name, err := test.name.NamedChild(test.scope, test.childName)
		if test.err != nil {
			c.Assert(err, tc.ErrorIs, test.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Check(test.output, tc.Equals, name)
	}
}
