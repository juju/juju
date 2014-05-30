// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"net"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type networkSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&networkSuite{})

type fakeAddress struct {
	address string
}

func (fake fakeAddress) Network() string {
	return "ignored"
}

func (fake fakeAddress) String() string {
	return fake.address
}

func makeAddresses(values ...string) (result []net.Addr) {
	for _, v := range values {
		result = append(result, &fakeAddress{v})
	}
	return
}

func (*networkSuite) TestGetIPv4Address(c *gc.C) {
	for _, test := range []struct {
		addresses   []net.Addr
		expected    string
		errorString string
	}{{
		addresses: makeAddresses(
			"complete",
			"nonsense"),
		errorString: "invalid CIDR address: complete",
	}, {
		addresses: makeAddresses(
			"fe80::90cf:9dff:fe6e:ece/64",
		),
		errorString: "no addresses match",
	}, {
		addresses: makeAddresses(
			"fe80::90cf:9dff:fe6e:ece/64",
			"10.0.3.1/24",
		),
		expected: "10.0.3.1",
	}, {
		addresses: makeAddresses(
			"10.0.3.1/24",
			"fe80::90cf:9dff:fe6e:ece/64",
		),
		expected: "10.0.3.1",
	}} {
		ip, err := utils.GetIPv4Address(test.addresses)
		if test.errorString == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(ip, gc.Equals, test.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errorString)
			c.Assert(ip, gc.Equals, "")
		}
	}
}
