// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	//"github.com/lxc/lxd"
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

var addrTests = []addrTest{{
	addr:     "",
	expected: "",
}, {
	// the typical remote case
	addr:     "https://x.y.z",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z:1234",
	expected: "https://x.y.z:1234",
}, {
	addr:     "https://x.y.z/:1234",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "https://x.y.z/a/b/c",
	expected: "https://x.y.z:8443",
}, {
	addr:     "http://x.y.z",
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
	addr:     "//x.y.z/a/b/c",
	expected: "unix://x.y.z",
}, {
	addr:     "/xyz/a/b/c",
	expected: "unix:///xyz/a/b/c",
}, {
	addr:     "x.y.z/a/b/c",
	exists:   true,
	expected: "unix://x.y.z/a/b/c",
}, {
	addr:     "xyz/a/b/c",
	exists:   true,
	expected: "unix://xyz/a/b/c",
}, {
	addr:     "x.y.z/a/b/c",
	exists:   false,
	expected: "https://x.y.z/a/b/c:8443",
}, {
	addr:     "xyz/a/b/c",
	exists:   false,
	expected: "https://xyz/a/b/c:8443",
}, {
	// the default local case
	addr:     "unix://",
	expected: "unix://",
}, {
	addr:     "unix:/",
	expected: "unix://",
}, {
	addr:     "unix://x/y/z",
	expected: "unix:///y/z",
}, {
	addr:     "unix:///x/y/z",
	expected: "unix://x/y/z",
}, {
	addr:     "unix:/x/y/z",
	expected: "unix://x/y/z",
}, {
	addr:     "unix:/x/y/z:8443",
	expected: "unix://[x/y/z:8443]",
}, {
	// a malformed URL
	addr: ":x.y.z",
	err:  `.*`,
}}

func (s *fixAddrSuite) TestFixAddr(c *gc.C) {
	for i, test := range addrTests {
		c.Logf("test %d: checking %q (exists: %v)", i, test.addr, test.exists)

		fixed, err := fixAddr(test.addr, test.pathExists)

		if test.err == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			c.Check(fixed, gc.Equals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
