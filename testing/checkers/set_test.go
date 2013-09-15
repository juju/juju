package checkers_test

import (
	"errors"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

type SetSuite struct{}

var _ = gc.Suite(&SetSuite{})

func (*SetSuite) TestSetInt(c *gc.C) {
	i := 99
	restore := jc.Set(&i, 88)
	c.Assert(i, gc.Equals, 88)
	restore()
	c.Assert(i, gc.Equals, 99)
}

func (*SetSuite) TestSetError(c *gc.C) {
	oldErr := errors.New("foo")
	newErr := errors.New("bar")
	err := oldErr
	restore := jc.Set(&err, newErr)
	c.Assert(err, gc.Equals, newErr)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*SetSuite) TestSetErrorToNil(c *gc.C) {
	oldErr := errors.New("foo")
	err := oldErr
	restore := jc.Set(&err, nil)
	c.Assert(err, gc.Equals, nil)
	restore()
	c.Assert(err, gc.Equals, oldErr)
}

func (*SetSuite) TestSetMapToNil(c *gc.C) {
	oldMap := map[string]int{"foo": 1234}
	m := oldMap
	restore := jc.Set(&m, nil)
	c.Assert(m, gc.IsNil)
	restore()
	c.Assert(m, gc.DeepEquals, oldMap)
}

func (*SetSuite) TestSetPanicsWhenNotAssignable(c *gc.C) {
	i := 99
	type otherInt int
	c.Assert(func() { jc.Set(&i, otherInt(88)) }, gc.PanicMatches, `reflect\.Set: value of type checkers_test\.otherInt is not assignable to type int`)
}
