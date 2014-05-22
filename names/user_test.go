// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type userSuite struct{}

var _ = gc.Suite(&userSuite{})

var validTests = []struct {
	string string
	expect bool
}{
	{"", false},
	{"bob", true},
	{"Bob", true},
	{"bOB", true},
	{"b^b", false},
	{"bob1", true},
	{"bob-1", true},
	{"bob+1", false},
	{"bob.1", true},
	{"1bob", false},
	{"1-bob", false},
	{"1+bob", false},
	{"1.bob", false},
	{"jim.bob+99-1.", false},
	{"a", false},
	{"0foo", false},
	{"foo bar", false},
	{"bar{}", false},
	{"bar+foo", false},
	{"bar_foo", false},
	{"bar!", false},
	{"bar^", false},
	{"bar*", false},
	{"foo=bar", false},
	{"foo?", false},
	{"[bar]", false},
	{"'foo'", false},
	{"%bar", false},
	{"&bar", false},
	{"#1foo", false},
	{"bar@ram.u", false},
	{"not/valid", false},
}

func (s *userSuite) TestUserTag(c *gc.C) {
	c.Assert(names.UserTag("admin"), gc.Equals, "user-admin")
}

func (s *userSuite) TestIsUser(c *gc.C) {
	for i, t := range validTests {
		c.Logf("test %d: %s", i, t.string)
		c.Assert(names.IsUser(t.string), gc.Equals, t.expect, gc.Commentf("%s", t.string))
	}
}
