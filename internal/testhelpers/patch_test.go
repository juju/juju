// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"errors"
	"os"

	"github.com/juju/tc"

	testing "github.com/juju/juju/internal/testhelpers"
)

type PatchValueSuite struct{}

var _ = tc.Suite(&PatchValueSuite{})

func (*PatchValueSuite) TestSetInt(c *tc.C) {
	i := 99
	restore := testing.PatchValue(&i, 88)
	c.Assert(i, tc.Equals, 88)
	restore()
	c.Assert(i, tc.Equals, 99)
}

func (*PatchValueSuite) TestSetError(c *tc.C) {
	oldErr := errors.New("foo")
	newErr := errors.New("bar")
	err := oldErr
	restore := testing.PatchValue(&err, newErr)
	c.Assert(err, tc.Equals, newErr)
	restore()
	c.Assert(err, tc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetErrorToNil(c *tc.C) {
	oldErr := errors.New("foo")
	err := oldErr
	restore := testing.PatchValue(&err, nil)
	c.Assert(err, tc.Equals, nil)
	restore()
	c.Assert(err, tc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetMapToNil(c *tc.C) {
	oldMap := map[string]int{"foo": 1234}
	m := oldMap
	restore := testing.PatchValue(&m, nil)
	c.Assert(m, tc.IsNil)
	restore()
	c.Assert(m, tc.DeepEquals, oldMap)
}

func (*PatchValueSuite) TestSetPanicsWhenNotAssignable(c *tc.C) {
	i := 99
	type otherInt int
	c.Assert(func() { testing.PatchValue(&i, otherInt(88)) }, tc.PanicMatches, `reflect\.Set: value of type testing_test\.otherInt is not assignable to type int`)
}

type PatchEnvironmentSuite struct{}

var _ = tc.Suite(&PatchEnvironmentSuite{})

func (*PatchEnvironmentSuite) TestPatchEnvironment(c *tc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and set it to something we can check
	oldValue, oldValueSet := os.LookupEnv(envName)
	defer func() {
		if oldValueSet {
			_ = os.Setenv(envName, oldValue)
		} else {
			_ = os.Unsetenv(envName)
		}
	}()

	_ = os.Setenv(envName, "initial")
	restore := testing.PatchEnvironment(envName, "new value")
	// Using check to make sure the environment gets set back properly in the test.
	c.Check(os.Getenv(envName), tc.Equals, "new value")
	restore()
	c.Check(os.Getenv(envName), tc.Equals, "initial")
}

func (*PatchEnvironmentSuite) TestPatchEnvironmentWithAbsentVar(c *tc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and unset the var
	oldValue, oldValueSet := os.LookupEnv(envName)
	defer func() {
		if oldValueSet {
			_ = os.Setenv(envName, oldValue)
		}
	}()

	_ = os.Unsetenv(envName)
	restore := testing.PatchEnvironment(envName, "new value")

	c.Check(os.Getenv(envName), tc.Equals, "new value")
	restore()
	_, set := os.LookupEnv(envName)
	c.Check(set, tc.Equals, false)
}

func (*PatchEnvironmentSuite) TestRestorerAdd(c *tc.C) {
	var order []string
	first := testing.Restorer(func() { order = append(order, "first") })
	second := testing.Restorer(func() { order = append(order, "second") })
	restore := first.Add(second)
	restore()
	c.Assert(order, tc.DeepEquals, []string{"second", "first"})
}

func (*PatchEnvironmentSuite) TestPatchEnvPathPrepend(c *tc.C) {
	oldPath := os.Getenv("PATH")
	dir := "/bin/bar"

	// just in case something goes wrong
	defer func() { _ = os.Setenv("PATH", oldPath) }()

	restore := testing.PatchEnvPathPrepend(dir)

	expect := dir + string(os.PathListSeparator) + oldPath
	c.Check(os.Getenv("PATH"), tc.Equals, expect)
	restore()
	c.Check(os.Getenv("PATH"), tc.Equals, oldPath)
}
