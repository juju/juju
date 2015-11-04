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
	addr:     "unix://",
	expected: "unix://",
}, {
	addr:     "a.b.c",
	exists:   false,
	expected: "https://a.b.c:8443",
}, {
	addr:     "https://a.b.c",
	expected: "https://a.b.c:8443",
}, {
	addr:     "https://a.b.c/",
	expected: "https://a.b.c:8443",
}, {
	addr:     "a.b.c:1234",
	exists:   false,
	expected: "https://a.b.c:1234",
}, {
	addr:     "https://a.b.c:1234",
	expected: "https://a.b.c:1234",
}, {
	addr:     "https://a.b.c:1234/",
	expected: "https://a.b.c:1234",
}, {
	addr:     "a.b.c/x/y/z",
	exists:   false,
	expected: "https://a.b.c:8443/x/y/z",
}, {
	addr:     "https://a.b.c/x/y/z",
	expected: "https://a.b.c:8443/x/y/z",
}, {
	addr:     "https://a.b.c:1234/x/y/z",
	expected: "https://a.b.c:1234/x/y/z",
}, {
	addr:     "http://a.b.c",
	expected: "https://a.b.c:8443",
}, {
	addr:     "1.2.3.4",
	expected: "https://1.2.3.4:8443",
}, {
	addr:     "1.2.3.4:1234",
	expected: "https://1.2.3.4:1234",
}, {
	addr:     "https://1.2.3.4",
	expected: "https://1.2.3.4:8443",
}, {
	addr:     "127.0.0.1",
	expected: "https://127.0.0.1:8443",
}, {
	addr:     "2001:db8::ff00:42:8329",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "[2001:db8::ff00:42:8329]:1234",
	expected: "https://[2001:db8::ff00:42:8329]:1234",
}, {
	addr:     "https://2001:db8::ff00:42:8329",
	expected: "https://[2001:db8::ff00:42:8329]:8443",
}, {
	addr:     "https://[2001:db8::ff00:42:8329]:1234",
	expected: "https://[2001:db8::ff00:42:8329]:1234",
}, {
	addr:     "::1",
	expected: "https://[::1]:8443",
}, {
	addr:     "unix:///x/y/z",
	exists:   true,
	expected: "unix:///x/y/z",
}, {
	addr:     "unix:/x/y/z",
	exists:   true,
	expected: "unix:///x/y/z",
}, {
	addr:     "/x/y/z",
	exists:   true,
	expected: "unix:///x/y/z",
}, {
	addr:     "a.b.c",
	exists:   false,
	expected: "https://a.b.c:8443",
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

// TODO(ericsnow) Add failure tests for bad domain names
// and IP addresses (v4/v6).

var failureAddrTests = []addrTest{{
	// a malformed URL
	addr: ":a.b.c",
	err:  `.*`,
}, {
	addr: "https://a.b.c:xyz",
	err:  `.*invalid port.*`,
}, {
	addr: "https://a.b.c:0",
	err:  `.*invalid port.*`,
}, {
	addr: "https://a.b.c:99999",
	err:  `.*invalid port.*`,
}, {
	addr: "spam://a.b.c",
	err:  `.*unsupported URL scheme.*`,
}, {
	addr: "https://a.b.c?d=e",
	err:  `.*URL queries not supported.*`,
}, {
	addr: "https://a.b.c#d",
	err:  `.*URL fragments not supported.*`,
}, {
	addr:   "/x/y/z",
	exists: false,
	err:    `.*unix socket file .* not found.*`,
}, {
	addr:   "x/y/z",
	exists: true,
	err:    `.*relative unix socket paths not supported \(got "x/y/z"\).*`,
}, {
	addr:   "//x/y/z",
	exists: true,
	err:    `.*relative unix socket paths not supported \(got "x/y/z"\).*`,
}, {
	addr:   "unix://x/y/z",
	exists: true,
	err:    `.*relative unix socket paths not supported \(got "x/y/z"\).*`,
}}

func (s *fixAddrSuite) TestFixAddrFailures(c *gc.C) {
	for i, test := range failureAddrTests {
		c.Logf("test %d: checking %q (exists: %v)", i, test.addr, test.exists)
		c.Assert(test.err, gc.Not(gc.Equals), "")

		_, err := fixAddr(test.addr, test.pathExists)

		c.Check(err, gc.ErrorMatches, test.err)
	}
}
