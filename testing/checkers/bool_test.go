// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"errors"
	. "launchpad.net/gocheck"
	. "launchpad.net/juju-core/testing/checkers"
	"os"
)

type BoolSuite struct{}

var _ = Suite(&BoolSuite{})

func (s *BoolSuite) TestIsTrue(c *C) {
	c.Assert(true, IsTrue)
	c.Assert(false, Not(IsTrue))

	result, msg := IsTrue.Check([]interface{}{false}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, "")

	result, msg = IsTrue.Check([]interface{}{"foo"}, nil)
	c.Assert(result, Equals, false)
	c.Check(msg, Equals, `expected type bool, received type string`)

	result, msg = IsTrue.Check([]interface{}{42}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, `expected type bool, received type int`)
}

func (s *BoolSuite) TestIsFalse(c *C) {
	c.Assert(false, IsFalse)
	c.Assert(true, Not(IsFalse))
}

func is42(i int) bool {
	return i == 42
}

func (s *BoolSuite) TestSatisfies(c *C) {
	result, msg := Satisfies.Check([]interface{}{42, is42}, nil)
	c.Assert(result, Equals, true)
	c.Assert(msg, Equals, "")

	result, msg = Satisfies.Check([]interface{}{41, is42}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, "")

	result, msg = Satisfies.Check([]interface{}{"", is42}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, "expected func(string) bool, got func(int) bool")

	result, msg = Satisfies.Check([]interface{}{"", is42}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, "expected func(string) bool, got func(int) bool")

	result, msg = Satisfies.Check([]interface{}{errors.New("foo"), os.IsNotExist}, nil)
	c.Assert(result, Equals, false)
	c.Assert(msg, Equals, "")

	result, msg = Satisfies.Check([]interface{}{os.ErrNotExist, os.IsNotExist}, nil)
	c.Assert(result, Equals, true)
	c.Assert(msg, Equals, "")
}
