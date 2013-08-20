// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type serviceSuite struct{}

var _ = gc.Suite(&serviceSuite{})

var serviceNameTests = []struct {
	pattern string
	valid   bool
}{
	{pattern: "", valid: false},
	{pattern: "wordpress", valid: true},
	{pattern: "foo42", valid: true},
	{pattern: "doing55in54", valid: true},
	{pattern: "%not", valid: false},
	{pattern: "42also-not", valid: false},
	{pattern: "but-this-works", valid: true},
	{pattern: "so-42-far-not-good", valid: false},
	{pattern: "foo/42", valid: false},
	{pattern: "is-it-", valid: false},
	{pattern: "broken2-", valid: false},
	{pattern: "foo2", valid: true},
	{pattern: "foo-2", valid: false},
}

func (s *serviceSuite) TestServiceNameFormats(c *gc.C) {
	assertService := func(s string, expect bool) {
		c.Assert(names.IsService(s), gc.Equals, expect)
		// Check that anything that is considered a valid service name
		// is also (in)valid if a(n) (in)valid unit designator is added
		// to it.
		c.Assert(names.IsUnit(s+"/0"), gc.Equals, expect)
		c.Assert(names.IsUnit(s+"/99"), gc.Equals, expect)
		c.Assert(names.IsUnit(s+"/-1"), gc.Equals, false)
		c.Assert(names.IsUnit(s+"/blah"), gc.Equals, false)
		c.Assert(names.IsUnit(s+"/"), gc.Equals, false)
	}

	for i, test := range serviceNameTests {
		c.Logf("test %d: %q", i, test.pattern)
		assertService(test.pattern, test.valid)
	}
}
