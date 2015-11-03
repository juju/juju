// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&fixAddrSuite{})

type fixAddrSuite struct {
	testing.IsolationSuite
}

type addrTest struct {
	addr     string
	exists   bool
	expected string
	err      string
}

func (t addrTest) pathExists(path string) bool {
	return t.exists
}

var typicalAddrTests = []addrTest{{
	addr:     "",
	expected: "",
}, {
	addr:     "https://x.y.z",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z:1234",
	expected: "https://x.y.z:1234",
}, {
	addr:     "http://x.y.z",
	expected: "https://x.y.z:8443",
}, {
	addr:     "1.2.3.4",
	expected: "https://1.2.3.4:8443",
}, {
	addr:     "https://1.2.3.4",
	expected: "https://1.2.3.4:8443",
}, {
	addr:     "2001:db8::ff00:42:8329",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "https://2001:db8::ff00:42:8329",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "https://[2001:db8::ff00:42:8329]:1234",
	expected: "https://[2001:db8::ff00:42:8329]:1234",
}, {
	addr:     "unix://",
	expected: "unix://",
}, {
	// TODO(ericsnow) Expect 3 slashes?
	addr:     "unix:///x/y/z",
	expected: "unix://x/y/z",
}, {
	// TODO(ericsnow) Expect 3 slashes?
	addr:     "unix:/x/y/z",
	expected: "unix://x/y/z",
}, {
	addr:     "/x/y/z",
	expected: "unix:///x/y/z",
}, {
	addr:     "x/y/z",
	exists:   true,
	expected: "unix://x/y/z",
}, {
	addr:     "x.y.z",
	exists:   false,
	expected: "https://x.y.z:8443",
}}

func (s *fixAddrSuite) TestFixAddrTypical(c *gc.C) {
	for i, test := range typicalAddrTests {
		c.Logf("test %d: checking %q (exists: %v)", i, test.addr, test.exists)
		c.Assert(test.err, gc.Equals, "")

		fixed, err := fixAddr(test.addr, test.pathExists)

		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		c.Check(fixed, gc.Equals, test.expected)
	}
}

var failureAddrTests = []addrTest{{
	// a malformed URL
	addr: ":x.y.z",
	err:  `.*`,
}}

func (s *fixAddrSuite) TestFixAddrFailures(c *gc.C) {
	for i, test := range failureAddrTests {
		c.Logf("test %d: checking %q (exists: %v)", i, test.addr, test.exists)
		c.Assert(test.err, gc.Not(gc.Equals), "")

		_, err := fixAddr(test.addr, test.pathExists)

		c.Check(err, gc.ErrorMatches, test.err)
	}
}

var weirdAddrTests = []addrTest{{
	addr:     "https://x.y.z/:1234",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "http://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "http://x.y.z/a/b/c:1234",
	expected: "https://x.y.z:8443",
}, {
	addr:     "spam://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "2001:db8::ff00:42:8329/a/b/c",
	expected: "https://[2001:db8::ff00:42:8329/a/b/c]:8443",
}, {
	addr:     "https://2001:db8::ff00:42:8329/a/b/c",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "https://[2001:db8::ff00:42:8329]/a/b/c",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "//a.b.c/x/y/z",
	expected: "unix://a.b.c",
}, {
	addr:     "a.b.c/x/y/z",
	exists:   true,
	expected: "unix://a.b.c/x/y/z",
}, {
	addr:     "x.y.z/a/b/c",
	exists:   false,
	expected: "https://x.y.z/a/b/c:8443",
}, {
	addr:     "xyz/a/b/c",
	exists:   false,
	expected: "https://xyz/a/b/c:8443",
}, {
	addr:     "unix:/",
	expected: "unix://",
}, {
	addr:     "unix://x/y/z",
	expected: "unix:///y/z",
}, {
	addr:     "unix:/x/y/z:8443",
	expected: "unix://[x/y/z:8443]",
}}

func (s *fixAddrSuite) TestFixAddrWeird(c *gc.C) {
	for i, test := range weirdAddrTests {
		c.Logf("test %d: checking %q (exists: %v)", i, test.addr, test.exists)
		c.Assert(test.err, gc.Equals, "")

		fixed, err := fixAddr(test.addr, test.pathExists)

		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		c.Check(fixed, gc.Equals, test.expected)
	}
}
