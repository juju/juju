// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	"fmt"
	"regexp"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type networkSuite struct{}

var _ = gc.Suite(&networkSuite{})

var networkNameTests = []struct {
	pattern string
	valid   bool
}{
	{pattern: "", valid: false},
	{pattern: "eth0", valid: true},
	{pattern: "-my-net-", valid: false},
	{pattern: "42", valid: true},
	{pattern: "%not", valid: false},
	{pattern: "$PATH", valid: false},
	{pattern: "but-this-works", valid: true},
	{pattern: "----", valid: false},
	{pattern: "oh--no", valid: false},
	{pattern: "777", valid: true},
	{pattern: "is-it-", valid: false},
	{pattern: "also_not", valid: false},
	{pattern: "a--", valid: false},
	{pattern: "foo-2", valid: true},
}

func (s *networkSuite) TestNetworkNames(c *gc.C) {
	for i, test := range networkNameTests {
		c.Logf("test %d: %q", i, test.pattern)
		c.Check(names.IsNetwork(test.pattern), gc.Equals, test.valid)
		if test.valid {
			expectTag := fmt.Sprintf("%s-%s", names.NetworkTagKind, test.pattern)
			c.Check(names.NetworkTag(test.pattern), gc.Equals, expectTag)
		} else {
			expectErr := fmt.Sprintf("%q is not a valid network name", test.pattern)
			testNetworkTag := func() { names.NetworkTag(test.pattern) }
			c.Check(testNetworkTag, gc.PanicMatches, regexp.QuoteMeta(expectErr))
		}
	}
}
