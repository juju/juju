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
}

func (s *serviceSuite) TestServiceNameFormats(c *gc.C) {
	for i, test := range serviceNameTests {
		c.Logf("test %d: %q", i, test.pattern)
		c.Assert(names.IsService(test.pattern), gc.Equals, test.valid)
	}
}
