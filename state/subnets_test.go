// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type SubnetSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SubnetSuite{})

func (s *SubnetSuite) TestAddSubnet(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		ProviderId:        "foo",
		CIDR:              "192.168.1.0/24",
		VLANTag:           79,
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: "192.168.1.1",
		AvailabilityZone:  "Timbuktu",
		SpaceName:         "foo",
	}

	assertSubnet := func(subnet *state.Subnet) {
		c.Assert(subnet.ProviderId(), gc.Equals, "foo")
		c.Assert(subnet.CIDR(), gc.Equals, "192.168.1.0/24")
		c.Assert(subnet.VLANTag(), gc.Equals, 79)
		c.Assert(subnet.AllocatableIPLow(), gc.Equals, "192.168.1.0")
		c.Assert(subnet.AllocatableIPHigh(), gc.Equals, "192.168.1.1")
		c.Assert(subnet.AvailabilityZone(), gc.Equals, "Timbuktu")
		c.Assert(subnet.String(), gc.Equals, "192.168.1.0/24")
		c.Assert(subnet.GoString(), gc.Equals, "192.168.1.0/24")
		c.Assert(subnet.SpaceName(), gc.Equals, "foo")
	}

	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	assertSubnet(subnet)

	// check it's been stored in state by fetching it back again
	subnetFromDB, err := s.State.Subnet("192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	assertSubnet(subnetFromDB)
}

func (s *SubnetSuite) TestAddSubnetErrors(c *gc.C) {
	subnetInfo := state.SubnetInfo{}
	_, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, `adding subnet "": missing CIDR`)

	subnetInfo.CIDR = "foobar"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches,
		`adding subnet "foobar": invalid CIDR address: foobar`,
	)

	errPrefix := `adding subnet "192.168.0.1/24": `
	subnetInfo.CIDR = "192.168.0.1/24"
	subnetInfo.VLANTag = 4095
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches,
		errPrefix+"invalid VLAN tag 4095: must be between 0 and 4094",
	)

	eitherOrMsg := errPrefix + "either both AllocatableIPLow and AllocatableIPHigh must be set or neither set"
	subnetInfo.VLANTag = 0
	subnetInfo.AllocatableIPHigh = "192.168.0.1"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, eitherOrMsg)

	subnetInfo.AllocatableIPLow = "192.168.0.1"
	subnetInfo.AllocatableIPHigh = ""
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, eitherOrMsg)

	// invalid IP address
	subnetInfo.AllocatableIPHigh = "foobar"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, errPrefix+`invalid AllocatableIPHigh "foobar"`)

	// invalid IP address
	subnetInfo.AllocatableIPLow = "foobar"
	subnetInfo.AllocatableIPHigh = "192.168.0.1"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, errPrefix+`invalid AllocatableIPLow "foobar"`)

	// IP address out of range
	subnetInfo.AllocatableIPHigh = "172.168.1.0"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, errPrefix+`invalid AllocatableIPHigh "172.168.1.0"`)

	// IP address out of range
	subnetInfo.AllocatableIPHigh = "192.168.0.1"
	subnetInfo.AllocatableIPLow = "172.168.1.0"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches, errPrefix+`invalid AllocatableIPLow "172.168.1.0"`)

	// valid case
	subnetInfo.AllocatableIPLow = "192.168.0.1"
	subnetInfo.ProviderId = "testing uniqueness"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)

	// ProviderId should be unique as well as CIDR
	subnetInfo.CIDR = "192.0.0.0/0"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, gc.ErrorMatches,
		`adding subnet "192.0.0.0/0": ProviderId "testing uniqueness" not unique`,
	)

	// empty provider id should be allowed to be not unique
	subnetInfo.ProviderId = ""
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	subnetInfo.CIDR = "192.0.0.1/1"
	_, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SubnetSuite) TestSubnetEnsureDeadRemove(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.1.0/24"}

	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, state.Alive)

	// This should fail - not dead yet!
	err = subnet.Remove()
	c.Assert(err, gc.ErrorMatches,
		`cannot remove subnet "192.168.1.0/24": subnet is not dead`,
	)

	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, state.Dead)

	// EnsureDead a second time should also not be an error
	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, state.Dead)

	// check the change was persisted
	subnetCopy, err := s.State.Subnet("192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetCopy.Life(), gc.Equals, state.Dead)

	// Remove should now work
	err = subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.Subnet("192.168.1.0/24")
	c.Assert(err, gc.ErrorMatches, `subnet "192.168.1.0/24" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// removing a second time should be a no-op
	err = subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SubnetSuite) TestSubnetRemoveKillsAddresses(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.1.0/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddIPAddress(
		network.NewAddress("192.168.1.0"),
		subnet.ID(),
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddIPAddress(
		network.NewAddress("192.168.1.1"),
		subnet.ID(),
	)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.IPAddress("192.168.1.0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.IPAddress("192.168.1.1")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SubnetSuite) TestRefresh(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.1.0/24"}

	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	subnetCopy, err := s.State.Subnet("192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnetCopy.Life(), gc.Equals, state.Alive)
	err = subnetCopy.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetCopy.Life(), gc.Equals, state.Dead)
}

func (s *SubnetSuite) TestAllSubnets(c *gc.C) {
	subnetInfos := []state.SubnetInfo{
		{CIDR: "192.168.1.0/24"},
		{CIDR: "8.8.8.0/24", SpaceName: "bar"},
		{CIDR: "10.0.2.0/24", ProviderId: "foo"},
		{CIDR: "2001:db8::/64", AvailabilityZone: "zone1"},
	}

	for _, info := range subnetInfos {
		_, err := s.State.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
	}

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, len(subnetInfos))

	for i, subnet := range subnets {
		c.Assert(subnet.CIDR(), gc.Equals, subnetInfos[i].CIDR)
		c.Assert(subnet.ProviderId(), gc.Equals, subnetInfos[i].ProviderId)
		c.Assert(subnet.SpaceName(), gc.Equals, subnetInfos[i].SpaceName)
		c.Assert(subnet.AvailabilityZone(), gc.Equals, subnetInfos[i].AvailabilityZone)
	}
}

func (s *SubnetSuite) TestPickNewAddressNoAddresses(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.1.0/24",
		AllocatableIPLow:  "",
		AllocatableIPHigh: "",
	}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, err = subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches, "no allocatable IP addresses for subnet .*")
}

func (s *SubnetSuite) getSubnetForAddressPicking(c *gc.C, allocatableHigh string) *state.Subnet {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.1.0/24",
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: allocatableHigh,
	}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	return subnet
}

func (s *SubnetSuite) TestPickNewAddressWhenSubnetIsDead(c *gc.C) {
	subnet := s.getSubnetForAddressPicking(c, "192.168.1.0")
	err := subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Calling it twice is ok.
	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	_, err = subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches,
		`cannot pick address: subnet "192.168.1.0/24" is not alive`,
	)
}

func (s *SubnetSuite) TestPickNewAddressAddressesExhausted(c *gc.C) {
	subnet := s.getSubnetForAddressPicking(c, "192.168.1.0")
	addr := network.NewAddress("192.168.1.0")
	_, err := s.State.AddIPAddress(addr, subnet.ID())

	_, err = subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches, "allocatable IP addresses exhausted for subnet .*")
}

func (s *SubnetSuite) TestPickNewAddressOneAddress(c *gc.C) {
	subnet := s.getSubnetForAddressPicking(c, "192.168.1.0")

	addr, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value(), gc.Equals, "192.168.1.0")
}

func (s *SubnetSuite) TestPickNewAddressSkipsAllocated(c *gc.C) {
	subnet := s.getSubnetForAddressPicking(c, "192.168.1.1")

	addr := network.NewAddress("192.168.1.0")
	_, err := s.State.AddIPAddress(addr, subnet.ID())

	ipAddr, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Value(), gc.Equals, "192.168.1.1")
}

func (s *SubnetSuite) TestPickNewAddressRace(c *gc.C) {
	// represents 192.168.1.0
	initialIP := uint32(3232235776)
	var index int32 = -1
	addresses := []uint32{initialIP, initialIP, initialIP + 1}

	// the first two calls will get the same address (which simulates the
	// inherent race condition in the code). The third call will get
	// a new one. We should see two different addresses come out of the
	// two calls: i.e. we will have detected the race condition and tried
	// again.
	mockPickAddress := func(_, _ uint32, _ map[uint32]bool) uint32 {
		theIndex := atomic.AddInt32(&index, 1)
		return addresses[theIndex]
	}
	s.PatchValue(&state.PickAddress, &mockPickAddress)

	// 192.168.1.0 and 192.168.1.1 are the only valid addresses
	subnet := s.getSubnetForAddressPicking(c, "192.168.1.1")

	waiter := sync.WaitGroup{}
	waiter.Add(2)

	var firstResult *state.IPAddress
	var firstError error
	var secondResult *state.IPAddress
	var secondError error
	go func() {
		firstResult, firstError = subnet.PickNewAddress()
		waiter.Done()
	}()
	go func() {
		secondResult, secondError = subnet.PickNewAddress()
		waiter.Done()
	}()
	waiter.Wait()

	c.Assert(firstError, jc.ErrorIsNil)
	c.Assert(secondError, jc.ErrorIsNil)
	c.Assert(firstResult, gc.NotNil)
	c.Assert(secondResult, gc.NotNil)

	ipAddresses := []string{firstResult.Value(), secondResult.Value()}
	sort.Strings(ipAddresses)

	expected := []string{"192.168.1.0", "192.168.1.1"}
	c.Assert(ipAddresses, jc.DeepEquals, expected)
}
