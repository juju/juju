// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"errors"
	"os"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

type BoolSuite struct{}

var _ = gc.Suite(&BoolSuite{})

func (s *BoolSuite) TestIsTrue(c *gc.C) {
	c.Assert(true, jc.IsTrue)
	c.Assert(false, gc.Not(jc.IsTrue))

	result, msg := jc.IsTrue.Check([]interface{}{false}, nil)
	c.Assert(result, gc.Equals, false)
	c.Assert(msg, gc.Equals, "")

	result, msg = jc.IsTrue.Check([]interface{}{"foo"}, nil)
	c.Assert(result, gc.Equals, false)
	c.Check(msg, gc.Equals, `expected type bool, received type string`)

	result, msg = jc.IsTrue.Check([]interface{}{42}, nil)
	c.Assert(result, gc.Equals, false)
	c.Assert(msg, gc.Equals, `expected type bool, received type int`)
}

func (s *BoolSuite) TestIsFalse(c *gc.C) {
	c.Assert(false, jc.IsFalse)
	c.Assert(true, gc.Not(jc.IsFalse))
}

func is42(i int) bool {
	return i == 42
}

var satisfiesTests = []struct {
	f      interface{}
	arg    interface{}
	result bool
	msg    string
}{{
	f:      is42,
	arg:    42,
	result: true,
}, {
	f:      is42,
	arg:    41,
	result: false,
}, {
	f:      is42,
	arg:    "",
	result: false,
	msg:    "wrong argument type string for func(int) bool",
}, {
	f:      os.IsNotExist,
	arg:    errors.New("foo"),
	result: false,
}, {
	f:      os.IsNotExist,
	arg:    os.ErrNotExist,
	result: true,
}, {
	f:      os.IsNotExist,
	arg:    nil,
	result: false,
}, {
	f:      func(chan int) bool { return true },
	arg:    nil,
	result: true,
}, {
	f:      func(func()) bool { return true },
	arg:    nil,
	result: true,
}, {
	f:      func(interface{}) bool { return true },
	arg:    nil,
	result: true,
}, {
	f:      func(map[string]bool) bool { return true },
	arg:    nil,
	result: true,
}, {
	f:      func(*int) bool { return true },
	arg:    nil,
	result: true,
}, {
	f:      func([]string) bool { return true },
	arg:    nil,
	result: true,
}}

func (s *BoolSuite) TestSatisfies(c *gc.C) {
	for i, test := range satisfiesTests {
		c.Logf("test %d. %T %T", i, test.f, test.arg)
		result, msg := jc.Satisfies.Check([]interface{}{test.arg, test.f}, nil)
		c.Check(result, gc.Equals, test.result)
		c.Check(msg, gc.Equals, test.msg)
	}
}

func (s *BoolSuite) TestDeepEquals(c *gc.C) {
	for i, test := range deepEqualTests {
		c.Logf("test %d. %v == %v is %v", i, test.a, test.b, test.eq)
		result, msg := jc.DeepEquals.Check([]interface{}{test.a, test.b}, nil)
		c.Check(result, gc.Equals, test.eq)
		if test.eq {
			c.Check(msg, gc.Equals, "")
		} else {
			c.Check(msg, gc.Not(gc.Equals), "")
		}
	}
}
