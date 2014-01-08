// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type relationSuite struct{}

var _ = gc.Suite(&relationSuite{})

var relationNameTests = []struct {
	pattern string
	valid   bool
}{
	{pattern: "", valid: false},
	{pattern: "0foo", valid: false},
	{pattern: "foo", valid: true},
	{pattern: "f1-boo", valid: true},
	{pattern: "f-o-o", valid: true},
	{pattern: "-foo", valid: false},
	{pattern: "fo#o", valid: false},
	{pattern: "foo-42", valid: true},
	{pattern: "FooBar", valid: false},
	{pattern: "foo42-bar1", valid: true},
	{pattern: "42", valid: false},
	{pattern: "0", valid: false},
	{pattern: "%not", valid: false},
	{pattern: "42also-not", valid: false},
	{pattern: "042", valid: false},
	{pattern: "0x42", valid: false},
	{pattern: "foo_42", valid: true},
	{pattern: "_foo", valid: false},
	{pattern: "!foo", valid: false},
	{pattern: "foo_bar-baz_boo", valid: true},
	{pattern: "foo bar", valid: false},
	{pattern: "foo-_", valid: false},
	{pattern: "foo-", valid: false},
	{pattern: "foo_-a", valid: false},
	{pattern: "foo_", valid: false},
}

func (s *relationSuite) TestRelationKeyFormats(c *gc.C) {
	// In order to test all possible combinations, we need
	// to use the relationNameTests and serviceNameTests
	// twice to construct all possible keys.
	for i, testRel := range relationNameTests {
		for j, testSvc := range serviceNameTests {
			peerKey := testSvc.pattern + ":" + testRel.pattern
			key := peerKey + " " + peerKey
			isValid := testSvc.valid && testRel.valid
			c.Logf("test %d: %q -> valid: %v", i*len(serviceNameTests)+j, key, isValid)
			c.Assert(names.IsRelation(key), gc.Equals, isValid)
			c.Assert(names.IsRelation(peerKey), gc.Equals, isValid)
		}
	}
}
