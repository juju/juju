// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase_test

import (
	"errors"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type PatchValueSuite struct{}

var _ = gc.Suite(&PatchValueSuite{})

func (*PatchValueSuite) TestSetInt(c *gc.C) {
	i := 99
	restore := testbase.PatchValue(&i, 88)
	c.Assert(i, gc.Equals, 88)
	restore()
	c.Assert(i, gc.Equals, 99)
}

func (*PatchValueSuite) TestSetError(c *gc.C) {
	oldErr := errors.New("foo")
	newErr := errors.New("bar")
	err := oldErr
	restore := testbase.PatchValue(&err, newErr)
	c.Assert(err, gc.Equals, newErr)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetErrorToNil(c *gc.C) {
	oldErr := errors.New("foo")
	err := oldErr
	restore := testbase.PatchValue(&err, nil)
	c.Assert(err, gc.Equals, nil)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetMapToNil(c *gc.C) {
	oldMap := map[string]int{"foo": 1234}
	m := oldMap
	restore := testbase.PatchValue(&m, nil)
	c.Assert(m, gc.IsNil)
	restore()
	c.Assert(m, gc.DeepEquals, oldMap)
}

func (*PatchValueSuite) TestSetPanicsWhenNotAssignable(c *gc.C) {
	i := 99
	type otherInt int
	c.Assert(func() { testbase.PatchValue(&i, otherInt(88)) }, gc.PanicMatches, `reflect\.Set: value of type testbase_test\.otherInt is not assignable to type int`)
}

type PatchEnvironmentSuite struct{}

var _ = gc.Suite(&PatchEnvironmentSuite{})

func (*PatchEnvironmentSuite) TestPatchEnvironment(c *gc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and set it to something we can check
	oldValue := os.Getenv(envName)
	os.Setenv(envName, "initial")
	restore := testbase.PatchEnvironment(envName, "new value")
	// Using check to make sure the environment gets set back properly in the test.
	c.Check(os.Getenv(envName), gc.Equals, "new value")
	restore()
	c.Check(os.Getenv(envName), gc.Equals, "initial")
	os.Setenv(envName, oldValue)
}

func (*PatchEnvironmentSuite) TestRestorerAdd(c *gc.C) {
	var order []string
	first := testbase.Restorer(func() { order = append(order, "first") })
	second := testbase.Restorer(func() { order = append(order, "second") })
	restore := first.Add(second)
	restore()
	c.Assert(order, gc.DeepEquals, []string{"second", "first"})
}

func (*PatchEnvironmentSuite) TestPatchEnvPathPrepend(c *gc.C) {
	oldPath := os.Getenv("PATH")
	dir := "/bin/bar"

	// just in case something goes wrong
	defer os.Setenv("PATH", oldPath)

	restore := testbase.PatchEnvPathPrepend(dir)

	expect := dir + string(os.PathListSeparator) + oldPath
	c.Check(os.Getenv("PATH"), gc.Equals, expect)
	restore()
	c.Check(os.Getenv("PATH"), gc.Equals, oldPath)
}
