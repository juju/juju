// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"flag"
	"testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"

	"launchpad.net/juju-core/provider/openstack"
)

var live = flag.Bool("live", false, "Include live OpenStack tests")

func Test(t *testing.T) {
	if *live {
		cred, err := identity.CompleteCredentialsFromEnv()
		if err != nil {
			t.Fatalf("Error setting up test suite: %s", err.Error())
		}
		registerLiveTests(cred)
	}
	registerLocalTests()
	gc.TestingT(t)
}

// localTests contains tests which do not require a live service or test double to run.
type localTests struct{}

var _ = gc.Suite(&localTests{})

// ported from lp:juju/juju/providers/openstack/tests/test_machine.py
var addressTests = []struct {
	summary  string
	private  []nova.IPAddress
	public   []nova.IPAddress
	networks []string
	expected string
	failure  error
}{
	{
		summary:  "missing",
		expected: "",
	},
	{
		summary:  "empty",
		private:  []nova.IPAddress{},
		networks: []string{"private"},
		expected: "",
	},
	{
		summary:  "private only",
		private:  []nova.IPAddress{{4, "192.168.0.1"}},
		networks: []string{"private"},
		expected: "192.168.0.1",
	},
	{
		summary:  "private plus (HP cloud)",
		private:  []nova.IPAddress{{4, "10.0.0.1"}, {4, "8.8.4.4"}},
		networks: []string{"private"},
		expected: "8.8.4.4",
	},
	{
		summary:  "public only",
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"", "public"},
		expected: "8.8.8.8",
	},
	{
		summary:  "public and private",
		private:  []nova.IPAddress{{4, "10.0.0.4"}},
		public:   []nova.IPAddress{{4, "8.8.4.4"}},
		networks: []string{"private", "public"},
		expected: "8.8.4.4",
	},
	{
		summary:  "public private plus",
		private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "192.168.0.1"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"private", "public"},
		expected: "8.8.8.8",
	},
	{
		summary:  "custom only",
		private:  []nova.IPAddress{{4, "192.168.0.1"}},
		networks: []string{"special"},
		expected: "192.168.0.1",
	},
	{
		summary:  "custom and public",
		private:  []nova.IPAddress{{4, "172.16.0.1"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"special", "public"},
		expected: "8.8.8.8",
	},
	{
		summary:  "non-IPv4",
		private:  []nova.IPAddress{{6, "::dead:beef:f00d"}},
		networks: []string{"private"},
		expected: "",
	},
}

func (t *localTests) TestGetServerAddresses(c *gc.C) {
	for i, t := range addressTests {
		c.Logf("#%d. %s -> %s (%v)", i, t.summary, t.expected, t.failure)
		addresses := make(map[string][]nova.IPAddress)
		if t.private != nil {
			if len(t.networks) < 1 {
				addresses["private"] = t.private
			} else {
				addresses[t.networks[0]] = t.private
			}
		}
		if t.public != nil {
			if len(t.networks) < 2 {
				addresses["public"] = t.public
			} else {
				addresses[t.networks[1]] = t.public
			}
		}
		addr := openstack.InstanceAddress(addresses)
		c.Assert(addr, gc.Equals, t.expected)
	}
}
