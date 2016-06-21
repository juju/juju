// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"net"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type HostnameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HostnameSuite{})

func (s *HostnameSuite) TestStringFQDN(c *gc.C) {
	hostname := rfc5424.Hostname{
		FQDN:      "a.b.org",
		StaticIP:  net.ParseIP("10.3.2.1"),
		Hostname:  "a",
		DynamicIP: net.ParseIP("2001:4860:0:2001::68"),
	}

	str := hostname.String()

	c.Check(str, gc.Equals, "a.b.org")
}

func (s *HostnameSuite) TestStringStaticIP(c *gc.C) {
	hostname := rfc5424.Hostname{
		StaticIP:  net.ParseIP("10.3.2.1"),
		Hostname:  "a",
		DynamicIP: net.ParseIP("2001:4860:0:2001::68"),
	}

	str := hostname.String()

	c.Check(str, gc.Equals, "10.3.2.1")
}

func (s *HostnameSuite) TestStringHostname(c *gc.C) {
	hostname := rfc5424.Hostname{
		Hostname:  "a",
		DynamicIP: net.ParseIP("2001:4860:0:2001::68"),
	}

	str := hostname.String()

	c.Check(str, gc.Equals, "a")
}

func (s *HostnameSuite) TestStringDynamicIP(c *gc.C) {
	hostname := rfc5424.Hostname{
		DynamicIP: net.ParseIP("2001:4860:0:2001::68"),
	}

	str := hostname.String()

	c.Check(str, gc.Equals, "2001:4860:0:2001::68")
}

func (s *HostnameSuite) TestStringZeroValue(c *gc.C) {
	var hostname rfc5424.Hostname

	str := hostname.String()

	c.Check(str, gc.Equals, "-")
}

func (s *HostnameSuite) TestValidateFQDNOkay(c *gc.C) {
	hostname := rfc5424.Hostname{
		FQDN: "a.b.org",
	}

	err := hostname.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestValidateStaticIPOkay(c *gc.C) {
	hostname := rfc5424.Hostname{
		StaticIP: net.ParseIP("10.3.2.1"),
	}

	err := hostname.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestValidateHostnameOkay(c *gc.C) {
	hostname := rfc5424.Hostname{
		Hostname: "a",
	}

	err := hostname.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestValidateDynamicIPOkay(c *gc.C) {
	hostname := rfc5424.Hostname{
		DynamicIP: net.ParseIP("2001:4860:0:2001::68"),
	}

	err := hostname.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestValidateZeroValue(c *gc.C) {
	var hostname rfc5424.Hostname

	err := hostname.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestValidateBadASCII(c *gc.C) {
	hostname := rfc5424.Hostname{
		Hostname: "\x09",
	}

	err := hostname.Validate()

	c.Check(err, gc.ErrorMatches, `must be printable US ASCII \(\\x09 at pos 0\)`)
}

func (s *HostnameSuite) TestValidateTooBig(c *gc.C) {
	hostname := rfc5424.Hostname{
		Hostname: strings.Repeat("x", 256),
	}

	err := hostname.Validate()

	c.Check(err, gc.ErrorMatches, `too big \(max 255\)`)
}
