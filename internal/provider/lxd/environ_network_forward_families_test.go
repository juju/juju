// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
)

type forwardFamilySuite struct{}

func TestForwardFamilySuite(t *testing.T) {
	tc.Run(t, &forwardFamilySuite{})
}

func (s *forwardFamilySuite) TestNetworkAndUplinkFamilies(c *tc.C) {
	for _, test := range []struct {
		name             string
		guest4, guest6   string
		routes4, routes6 string
		expected         []int
		errorMatch       string
	}{
		{name: "IPv4 only", guest4: "10.0.0.1/24", guest6: "none", routes4: "192.0.2.0/24", expected: []int{4}},
		{name: "IPv6 only", guest4: "none", guest6: "fd42::1/64", routes6: "2001:db8::/64", expected: []int{6}},
		{name: "dual stack", guest4: "10.0.0.1/24", guest6: "fd42::1/64", routes4: "192.0.2.0/24", routes6: "2001:db8::/64", expected: []int{4, 6}},
		{name: "dual stack with IPv4 uplink", guest4: "10.0.0.1/24", guest6: "fd42::1/64", routes4: "192.0.2.0/24", expected: []int{4}},
		{name: "dual stack with IPv6 uplink", guest4: "10.0.0.1/24", guest6: "fd42::1/64", routes6: "2001:db8::/64", expected: []int{6}},
		{name: "IPv4 guest with dual stack uplink", guest4: "10.0.0.1/24", guest6: "none", routes4: "192.0.2.0/24", routes6: "2001:db8::/64", expected: []int{4}},
		{name: "multiple routes", guest4: "10.0.0.1/24", routes4: " 192.0.2.0/25,192.0.2.128/25 ", expected: []int{4}},
		{name: "no compatible family", guest4: "10.0.0.1/24", guest6: "none", routes6: "2001:db8::/64", errorMatch: `.*no address family with a permitted external allocation range`},
		{name: "no external ranges", guest4: "10.0.0.1/24", guest6: "fd42::1/64", errorMatch: `.*no address family with a permitted external allocation range`},
		{name: "no guest subnets", guest4: "none", guest6: "none", routes4: "192.0.2.0/24", errorMatch: `.*no address family with a permitted external allocation range`},
		{name: "invalid guest subnet", guest4: "invalid", routes4: "192.0.2.0/24", errorMatch: `invalid ipv4.address .*`},
		{name: "wrong guest family", guest4: "fd42::1/64", routes4: "192.0.2.0/24", errorMatch: `invalid ipv4.address .*`},
		{name: "invalid external subnet", guest4: "10.0.0.1/24", routes4: "invalid", errorMatch: `.*parsing external allocation range.*`},
	} {
		c.Run(test.name, func(t *testing.T) {
			c := &tc.TBC{TB: t}
			srv := NewMockServer(gomock.NewController(c))
			srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{}, nil)
			srv.EXPECT().GetProject("default").Return(&api.Project{}, "", nil)
			srv.EXPECT().GetNetworkInProject("UPLINK", "default").Return(&api.Network{Config: map[string]string{
				"ipv4.routes": test.routes4, "ipv6.routes": test.routes6,
			}}, "", nil)
			families, err := ovnForwardFamilies(c.Context(), srv, map[string]api.Network{
				"ovn0": {Config: map[string]string{"network": "UPLINK", "ipv4.address": test.guest4, "ipv6.address": test.guest6}},
			})
			if test.errorMatch != "" {
				c.Assert(err, tc.ErrorMatches, test.errorMatch)
				return
			}
			c.Assert(err, tc.ErrorIsNil)
			var versions []int
			for _, family := range families["ovn0"] {
				versions = append(versions, family.version)
			}
			c.Check(versions, tc.DeepEquals, test.expected)
		})
	}
}

func (s *forwardFamilySuite) TestRestrictedProject(c *tc.C) {
	for _, test := range []struct {
		name, restrictions string
		expected           []int
		errorMatch         string
	}{
		{name: "IPv6 restriction", restrictions: "UPLINK:2001:db8::/64", expected: []int{6}},
		{name: "IPv4 restriction", restrictions: "UPLINK:192.0.2.0/24", expected: []int{4}},
		{name: "dual stack restrictions", restrictions: " UPLINK:192.0.2.0/24, UPLINK:2001:db8::/64 ", expected: []int{4, 6}},
		{name: "ignore other uplinks", restrictions: "other:192.0.2.0/24,UPLINK:2001:db8::/64", expected: []int{6}},
		{name: "no range for uplink", restrictions: "other:192.0.2.0/24", errorMatch: `.*no address family with a permitted external allocation range`},
		{name: "malformed restriction", restrictions: "UPLINK", errorMatch: `.*invalid project subnet restriction.*`},
		{name: "malformed subnet", restrictions: "UPLINK:invalid", errorMatch: `.*parsing external allocation range.*`},
	} {
		c.Run(test.name, func(t *testing.T) {
			c := &tc.TBC{TB: t}
			srv := NewMockServer(gomock.NewController(c))
			srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{Project: "tenant"}, nil)
			srv.EXPECT().GetProject("tenant").Return(&api.Project{Config: map[string]string{
				"features.networks": "true", "restricted": "true", "restricted.networks.subnets": test.restrictions,
			}}, "", nil)
			// Restricted subnets are sufficient, without read access to uplinks
			// in the default project. Never fall back to unrestricted routes.
			families, err := ovnForwardFamilies(c.Context(), srv, dualStackNetworks())
			if test.errorMatch != "" {
				c.Assert(err, tc.ErrorMatches, test.errorMatch)
				return
			}
			c.Assert(err, tc.ErrorIsNil)
			var versions []int
			for _, family := range families["ovn0"] {
				versions = append(versions, family.version)
			}
			c.Check(versions, tc.DeepEquals, test.expected)
		})
	}
}

func (s *forwardFamilySuite) TestProjectWithOwnNetworksAndUnrestrictedUplink(c *tc.C) {
	srv := NewMockServer(gomock.NewController(c))
	srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{Project: "tenant"}, nil)
	srv.EXPECT().GetProject("tenant").Return(&api.Project{Config: map[string]string{
		"features.networks": "true", "restricted": "false", "restricted.networks.subnets": "UPLINK:2001:db8::/64",
	}}, "", nil)
	// Restrictions on an unrestricted project do not apply. The actual
	// uplink must be fetched from default, not from the instance's project.
	srv.EXPECT().GetNetworkInProject("UPLINK", "default").Return(&api.Network{Config: map[string]string{
		"ipv4.routes": "192.0.2.0/24",
	}}, "", nil)
	families, err := ovnForwardFamilies(c.Context(), srv, dualStackNetworks())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(families["ovn0"], tc.HasLen, 1)
	c.Check(families["ovn0"][0].version, tc.Equals, 4)
}

func (s *forwardFamilySuite) TestInheritedNetworksAndSharedUplink(c *tc.C) {
	srv := NewMockServer(gomock.NewController(c))
	srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{Project: "tenant"}, nil)
	srv.EXPECT().GetProject("tenant").Return(&api.Project{Config: map[string]string{
		"features.networks": "false",
	}}, "", nil)
	srv.EXPECT().GetProject("default").Return(&api.Project{}, "", nil)
	// Both OVN networks use one uplink, which is fetched only once.
	srv.EXPECT().GetNetworkInProject("UPLINK", "default").Return(&api.Network{Config: map[string]string{
		"ipv6.routes": "2001:db8::/64",
	}}, "", nil)
	networks := dualStackNetworks()
	networks["ovn1"] = networks["ovn0"]
	families, err := ovnForwardFamilies(c.Context(), srv, networks)
	c.Assert(err, tc.ErrorIsNil)
	for _, name := range []string{"ovn0", "ovn1"} {
		c.Assert(families[name], tc.HasLen, 1)
		c.Check(families[name][0].version, tc.Equals, 6)
	}
}

func (s *forwardFamilySuite) TestLookupErrors(c *tc.C) {
	for _, failingCall := range []string{"connection", "project", "uplink"} {
		c.Run(failingCall, func(t *testing.T) {
			c := &tc.TBC{TB: t}
			srv := NewMockServer(gomock.NewController(c))
			failure := errors.New("permission denied")
			switch failingCall {
			case "connection":
				srv.EXPECT().GetConnectionInfo().Return(nil, failure)
			case "project":
				srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{}, nil)
				srv.EXPECT().GetProject("default").Return(nil, "", failure)
			case "uplink":
				srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{}, nil)
				srv.EXPECT().GetProject("default").Return(&api.Project{}, "", nil)
				srv.EXPECT().GetNetworkInProject("UPLINK", "default").Return(nil, "", failure)
			}
			_, err := ovnForwardFamilies(c.Context(), srv, dualStackNetworks())
			c.Assert(err, tc.ErrorIs, failure)
		})
	}
}

func (s *forwardFamilySuite) TestCancelledDuringLookup(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	srv := NewMockServer(gomock.NewController(c))
	srv.EXPECT().GetConnectionInfo().Return(&lxdclient.ConnectionInfo{}, nil)
	srv.EXPECT().GetProject("default").Return(&api.Project{}, "", nil)
	srv.EXPECT().GetNetworkInProject("UPLINK", "default").DoAndReturn(func(string, string) (*api.Network, string, error) {
		cancel()
		return &api.Network{Config: map[string]string{"ipv4.routes": "192.0.2.0/24"}}, "", nil
	})
	_, err := ovnForwardFamilies(ctx, srv, dualStackNetworks())
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func dualStackNetworks() map[string]api.Network {
	return map[string]api.Network{"ovn0": {Config: map[string]string{
		"network": "UPLINK", "ipv4.address": "10.0.0.1/24", "ipv6.address": "fd42::1/64",
	}}}
}
