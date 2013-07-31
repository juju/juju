// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"testing"

	. "launchpad.net/gocheck"
	. "launchpad.net/juju-core/testing/checkers"
)

func Test(t *testing.T) { TestingT(t) }

type CheckerSuite struct{}

var _ = Suite(&CheckerSuite{})

func (s *CheckerSuite) TestHasPrefix(c *C) {
	c.Assert("foo bar", HasPrefix, "foo")
	c.Assert("foo bar", Not(HasPrefix), "omg")
}

func (s *CheckerSuite) TestHasSuffix(c *C) {
	c.Assert("foo bar", HasSuffix, "bar")
	c.Assert("foo bar", Not(HasSuffix), "omg")
}

func (s *CheckerSuite) TestContains(c *C) {
	c.Assert("foo bar baz", Contains, "foo")
	c.Assert("foo bar baz", Contains, "bar")
	c.Assert("foo bar baz", Contains, "baz")
	c.Assert("foo bar baz", Not(Contains), "omg")
}
