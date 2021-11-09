// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v4/neutron"
	"github.com/go-goose/goose/v4/nova"
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
)

type networkingSuite struct {
	jujutesting.IsolationSuite

	base    *MockNetworkingBase
	neutron *MockNetworkingNeutron
	nova    *MockNetworkingNova
	client  *MockNetworkingAuthenticatingClient
	ecfg    *MockNetworkingEnvironConfig

	serverAZ        string
	externalNetwork string
	ip              string
	ip2             string
	ip3             string
}

var _ = gc.Suite(&networkingSuite{})

func (s *networkingSuite) SetUpTest(c *gc.C) {
	s.serverAZ = "test-me"
	s.externalNetwork = "ext-net"
	s.ip = "10.4.5.6"
	s.ip2 = "10.4.5.42"
	s.ip3 = "10.4.5.75"
}

func (s *networkingSuite) TestAllocatePublicIPConfiguredExternalNetwork(c *gc.C) {
	// Get a FIP for an instance with a configured external-network,
	// which has available FIPs.  Other external networks to exist,
	// at last 1 in the same AZ as the instance. Should get the FIP
	// on the configured external-network.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2FromConfig()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fip, gc.NotNil)
	c.Assert(*fip, gc.Equals, s.ip)
}

func (s *networkingSuite) TestAllocatePublicIPUnconfiguredExternalNetwork(c *gc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP.  The first external network in the list
	// does not have an available FIP.  No configured external-networks.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2NotFromConfig()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fip, gc.NotNil)
	c.Assert(*fip, gc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPUnconfiguredExternalNetworkMultiAZ(c *gc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP. This external network exists in multiple
	// AZ, the one we want is not first in the list. The first external
	// network in the list does not have an available FIP.  No configured
	// external-networks.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2NotFromConfig()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2MultiAZ() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fip, gc.NotNil)
	c.Assert(*fip, gc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPFail(c *gc.C) {
	// Find external-networks, but none have an available FIP, nor
	// are they able to create one.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2Empty()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()
	s.expectAllocateFloatingIPV2FailAll()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(fip, gc.IsNil)
}

func (s *networkingSuite) TestAllocatePublicIPEmtpyAZEqualEmptyString(c *gc.C) {
	// Test for lp: 1891227 fix.  An empty slice for AZ should be
	// treated as an empty string AZ.
	s.serverAZ = ""
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectExternalNetwork()
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:   "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name: "test-me",
		},
	}, nil).AnyTimes()
	s.expectListFloatingIPsV2NotFromConfig()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fip, gc.NotNil)
	c.Assert(*fip, gc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPNoneAvailable(c *gc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP.  No FIPs are available in the configured
	// external network, so allocate one.  The first network fails to
	// allocate, the 2nd succeeds.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2FromConfigInUse()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2() // findNetworkAZForHostAddrs()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()
	s.expectAllocateFloatingIPV2()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fip, gc.NotNil)
	c.Assert(*fip, gc.Equals, s.ip3)
}

func (s *networkingSuite) TestAllocatePublicIPFailNoNetworkInAZ(c *gc.C) {
	// No external network in same AZ as the instance is found, no
	// external network is configured.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListInternalNetworksV2()        // findNetworkAZForHostAddrs()
	s.expectListExternalNetworksV2NotInAZ() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(fip, gc.IsNil)
}

func (s *networkingSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.neutron = NewMockNetworkingNeutron(ctrl)
	s.nova = NewMockNetworkingNova(ctrl)
	s.client = NewMockNetworkingAuthenticatingClient(ctrl)
	s.ecfg = NewMockNetworkingEnvironConfig(ctrl)

	s.base = NewMockNetworkingBase(ctrl)
	bExp := s.base.EXPECT()
	bExp.client().Return(s.client).AnyTimes()
	bExp.neutron().Return(s.neutron).AnyTimes()
	bExp.nova().Return(s.nova)
	bExp.ecfg().Return(s.ecfg)

	s.client.EXPECT().TenantId().Return("TenantId").AnyTimes()
	s.nova.EXPECT().GetServer(gomock.Any()).Return(&nova.ServerDetail{
		Addresses: map[string][]nova.IPAddress{
			"int-net": {},
		},
		AvailabilityZone: s.serverAZ,
	}, nil)

	return ctrl
}

func (s *networkingSuite) runAllocatePublicIP() (*string, error) {
	networking := &NeutronNetworking{NetworkingBase: s.base}
	return networking.AllocatePublicIP(instance.Id("32"))
}

func (s *networkingSuite) expectListFloatingIPsV2FromConfig() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{
		{FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1ddeadbeef", IP: s.ip},
	}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2FromConfigInUse() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{{
		FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1ddeadbeef",
		FixedIP:           "10.7.8.9",
		IP:                s.ip,
	}}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2NotFromConfig() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{
		{FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1d0d06f00d", IP: s.ip2},
	}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2Empty() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{}, nil)
}

func (s *networkingSuite) expectExternalNetwork() {
	s.ecfg.EXPECT().externalNetwork().Return(s.externalNetwork)
}

func (s *networkingSuite) expectListExternalNetworksV2() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:                "deadbeef-0bad-400d-8000-4b1ddeadbeef",
			Name:              s.externalNetwork,
			External:          true,
			AvailabilityZones: []string{s.serverAZ},
		}, {
			Name:              "do-not-pick-me",
			External:          true,
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			External:          true,
			AvailabilityZones: []string{s.serverAZ},
		},
	}, nil)
}

func (s *networkingSuite) expectListInternalNetworksV2() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:                "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			Name:              "int-net",
			AvailabilityZones: []string{s.serverAZ},
		}, {
			Name:              "internal-do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d8273450d",
			Name:              "unconfigured-int-net",
			AvailabilityZones: []string{s.serverAZ},
		},
	}, nil)
}

func (s *networkingSuite) expectListExternalNetworksV2MultiAZ() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Name:              "do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			AvailabilityZones: []string{"other", s.serverAZ},
		},
	}, nil).AnyTimes()
}

func (s *networkingSuite) expectListExternalNetworksV2NotInAZ() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Name:              "do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			AvailabilityZones: []string{"other"},
		},
	}, nil).AnyTimes()
}

func (s *networkingSuite) expectAllocateFloatingIPV2() {
	s.neutron.EXPECT().AllocateFloatingIPV2("deadbeef-0bad-400d-8000-4b1ddeadbeef").Return(nil, errors.NotFoundf("fip"))
	s.neutron.EXPECT().AllocateFloatingIPV2("deadbeef-0bad-400d-8000-4b1d0d06f00d").Return(&neutron.FloatingIPV2{
		FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		IP:                s.ip3,
	}, nil)
}

func (s *networkingSuite) expectAllocateFloatingIPV2FailAll() {
	s.neutron.EXPECT().AllocateFloatingIPV2(gomock.Any()).Return(nil, errors.NotFoundf("fip")).AnyTimes()
}
