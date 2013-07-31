// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

func Test(t *testing.T) { gc.TestingT(t) }

type CheckerSuite struct{}

var _ = gc.Suite(&CheckerSuite{})

func (s *CheckerSuite) TestHasPrefix(c *gc.C) {
	c.Assert("foo bar", jc.HasPrefix, "foo")
	c.Assert("foo bar", gc.Not(jc.HasPrefix), "omg")
}

func (s *CheckerSuite) TestHasSuffix(c *gc.C) {
	c.Assert("foo bar", jc.HasSuffix, "bar")
	c.Assert("foo bar", gc.Not(jc.HasSuffix), "omg")
}

func (s *CheckerSuite) TestContains(c *gc.C) {
	c.Assert("foo bar baz", jc.Contains, "foo")
	c.Assert("foo bar baz", jc.Contains, "bar")
	c.Assert("foo bar baz", jc.Contains, "baz")
	c.Assert("foo bar baz", gc.Not(jc.Contains), "omg")
}
