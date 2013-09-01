// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type relationSuite struct{}

var _ = gc.Suite(&relationSuite{})

var relationIdTests = []struct {
	pattern string
	valid   bool
}{
	{pattern: "", valid: false},
	{pattern: "foo", valid: false},
	{pattern: "foo42", valid: false},
	{pattern: "42", valid: true},
	{pattern: "0", valid: true},
	{pattern: "%not", valid: false},
	{pattern: "42also-not", valid: false},
	{pattern: "042", valid: false},
	{pattern: "0x42", valid: false},
}

func (s *relationSuite) TestRelationIdFormats(c *gc.C) {
	for i, test := range relationIdTests {
		c.Logf("test %d: %q", i, test.pattern)
		c.Assert(names.IsRelation(test.pattern), gc.Equals, test.valid)
	}
}
