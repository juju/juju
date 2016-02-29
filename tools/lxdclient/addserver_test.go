// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

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
	expected string
	err      string
}

var typicalAddrTests = []addrTest{{
	addr:     "",
	expected: "",
}, {
	addr:     "a.b.c",
	expected: "https://a.b.c:8443",
}, {
	addr:     "https://a.b.c",
	expected: "https://a.b.c:8443",
}, {
	addr:     "https://a.b.c/",
	expected: "https://a.b.c:8443",
}, {
	addr:     "a.b.c:1234",
	expected: "https://a.b.c:1234",
}, {
	addr:     "https://a.b.c:1234",
	expected: "https://a.b.c:1234",
}, {
	addr:     "https://a.b.c:1234/",
	expected: "https://a.b.c:1234",
}, {
	addr:     "a.b.c/x/y/z",
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
	addr:     "a.b.c",
	expected: "https://a.b.c:8443",
}}

func (s *fixAddrSuite) TestFixAddrTypical(c *gc.C) {
	for i, test := range typicalAddrTests {
		c.Logf("test %d: checking %q", i, test.addr)
		c.Assert(test.err, gc.Equals, "")

		fixed, err := fixAddr(test.addr)

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
	addr: "unix://",
	err:  `.*unix socket URLs not supported.*`,
}, {
	addr: "unix:///x/y/z",
	err:  `.*unix socket URLs not supported.*`,
}, {
	addr: "unix:/x/y/z",
	err:  `.*unix socket URLs not supported.*`,
}, {
	addr: "/x/y/z",
	err:  `.*unix socket URLs not supported.*`,
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
}}

func (s *fixAddrSuite) TestFixAddrFailures(c *gc.C) {
	for i, test := range failureAddrTests {
		c.Logf("test %d: checking %q", i, test.addr)
		c.Assert(test.err, gc.Not(gc.Equals), "")

		_, err := fixAddr(test.addr)

		c.Check(err, gc.ErrorMatches, test.err)
	}
}
