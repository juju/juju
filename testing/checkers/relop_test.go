// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	. "launchpad.net/gocheck"
	. "launchpad.net/juju-core/testing/checkers"
)

type RelopSuite struct{}

var _ = Suite(&RelopSuite{})

func (s *RelopSuite) TestGreaterThan(c *C) {
	c.Assert(45, GreaterThan, 42)
	c.Assert(2.25, GreaterThan, 1.0)
	c.Assert(42, Not(GreaterThan), 42)
	c.Assert(10, Not(GreaterThan), 42)

	result, msg := GreaterThan.Check([]interface{}{"Hello", "World"}, nil)
	c.Assert(result, IsFalse)
	c.Assert(msg, Equals, `obtained value string:"Hello" not supported`)
}

func (s *RelopSuite) TestLessThan(c *C) {
	c.Assert(42, LessThan, 45)
	c.Assert(1.0, LessThan, 2.25)
	c.Assert(42, Not(LessThan), 42)
	c.Assert(42, Not(LessThan), 10)

	result, msg := LessThan.Check([]interface{}{"Hello", "World"}, nil)
	c.Assert(result, IsFalse)
	c.Assert(msg, Equals, `obtained value string:"Hello" not supported`)
}
