// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

type RelopSuite struct{}

var _ = gc.Suite(&RelopSuite{})

func (s *RelopSuite) TestGreaterThan(c *gc.C) {
	c.Assert(45, jc.GreaterThan, 42)
	c.Assert(2.25, jc.GreaterThan, 1.0)
	c.Assert(42, gc.Not(jc.GreaterThan), 42)
	c.Assert(10, gc.Not(jc.GreaterThan), 42)

	result, msg := jc.GreaterThan.Check([]interface{}{"Hello", "World"}, nil)
	c.Assert(result, jc.IsFalse)
	c.Assert(msg, gc.Equals, `obtained value string:"Hello" not supported`)
}

func (s *RelopSuite) TestLessThan(c *gc.C) {
	c.Assert(42, jc.LessThan, 45)
	c.Assert(1.0, jc.LessThan, 2.25)
	c.Assert(42, gc.Not(jc.LessThan), 42)
	c.Assert(42, gc.Not(jc.LessThan), 10)

	result, msg := jc.LessThan.Check([]interface{}{"Hello", "World"}, nil)
	c.Assert(result, jc.IsFalse)
	c.Assert(msg, gc.Equals, `obtained value string:"Hello" not supported`)
}
