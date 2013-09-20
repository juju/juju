package testing_test

import (
	"errors"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type PatchValueSuite struct{}

var _ = gc.Suite(&PatchValueSuite{})

func (*PatchValueSuite) TestSetInt(c *gc.C) {
	i := 99
	restore := testing.PatchValue(&i, 88)
	c.Assert(i, gc.Equals, 88)
	restore()
	c.Assert(i, gc.Equals, 99)
}

func (*PatchValueSuite) TestSetError(c *gc.C) {
	oldErr := errors.New("foo")
	newErr := errors.New("bar")
	err := oldErr
	restore := testing.PatchValue(&err, newErr)
	c.Assert(err, gc.Equals, newErr)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetErrorToNil(c *gc.C) {
	oldErr := errors.New("foo")
	err := oldErr
	restore := testing.PatchValue(&err, nil)
	c.Assert(err, gc.Equals, nil)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*PatchValueSuite) TestSetMapToNil(c *gc.C) {
	oldMap := map[string]int{"foo": 1234}
	m := oldMap
	restore := testing.PatchValue(&m, nil)
	c.Assert(m, gc.IsNil)
	restore()
	c.Assert(m, gc.DeepEquals, oldMap)
}

func (*PatchValueSuite) TestSetPanicsWhenNotAssignable(c *gc.C) {
	i := 99
	type otherInt int
	c.Assert(func() { testing.PatchValue(&i, otherInt(88)) }, gc.PanicMatches, `reflect\.Set: value of type testing_test\.otherInt is not assignable to type int`)
}

type PatchEnvironmentSuite struct{}

var _ = gc.Suite(&PatchEnvironmentSuite{})

func (*PatchEnvironmentSuite) TestPatchEnvironment(c *gc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and set it to something we can check
	oldValue := os.Getenv(envName)
	os.Setenv(envName, "initial")
	restore := testing.PatchEnvironment(envName, "new value")
	// Using check to make sure the environment gets set back properly in the test.
	c.Check(os.Getenv(envName), gc.Equals, "new value")
	restore()
	c.Check(os.Getenv(envName), gc.Equals, "initial")
	os.Setenv(envName, oldValue)
}
