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
		{
			name: "0/lxd/0",
		},
		{
			name: "0/lxd/0/lxd/0",
			err:  coreerrors.NotValid,
		},
		{
			name: "0/lxd/0/",
			err:  coreerrors.NotValid,
		},
		{
			name: "0//0",
			err:  coreerrors.NotValid,
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
			continue
		}

		c.Assert(err, tc.ErrorIsNil)
		c.Check(test.output, tc.Equals, name)

		parent := name.Parent()
		c.Check(parent, tc.Equals, test.name)

		child := name.Child()
		c.Check(child, tc.Equals, Name(test.childName))
	}
}

func (*machineSuite) TestNamedParentRelationship(c *tc.C) {
	tests := []struct {
		name        Name
		parent      Name
		child       Name
		isContainer bool
	}{
		{
			name:        Name("0/lxd/1"),
			parent:      Name("0"),
			child:       Name("1"),
			isContainer: true,
		},
		{
			name: func() Name {
				n, _ := Name("0").NamedChild("lxd", "2")
				return n
			}(),
			parent:      Name("0"),
			child:       Name("2"),
			isContainer: true,
		},
		{
			name:        "foo",
			parent:      "foo",
			child:       "foo",
			isContainer: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.name)
		c.Check(test.name.IsContainer(), tc.Equals, test.isContainer)

		c.Check(test.name.Parent(), tc.Equals, test.parent)
		c.Check(test.name.Child(), tc.Equals, test.child)
	}
}
