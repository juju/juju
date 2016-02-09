// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
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

func (s *SubnetSuite) TestAddSubnetSucceedsWithFullyPopulatedInfo(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		ProviderId:        "foo",
		CIDR:              "192.168.1.0/24",
		VLANTag:           79,
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: "192.168.1.1",
		AvailabilityZone:  "Timbuktu",
		SpaceName:         "foo",
	}

	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	// check it's been stored in state by fetching it back again
	subnetFromDB, err := s.State.Subnet("192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnetFromDB, subnetInfo)
}

func (s *SubnetSuite) assertSubnetMatchesInfo(c *gc.C, subnet *state.Subnet, info state.SubnetInfo) {
	c.Assert(subnet.ProviderId(), gc.Equals, info.ProviderId)
	c.Assert(subnet.CIDR(), gc.Equals, info.CIDR)
	c.Assert(subnet.VLANTag(), gc.Equals, info.VLANTag)
	c.Assert(subnet.AllocatableIPLow(), gc.Equals, info.AllocatableIPLow)
	c.Assert(subnet.AllocatableIPHigh(), gc.Equals, info.AllocatableIPHigh)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, info.AvailabilityZone)
	c.Assert(subnet.String(), gc.Equals, info.CIDR)
	c.Assert(subnet.GoString(), gc.Equals, info.CIDR)
	c.Assert(subnet.SpaceName(), gc.Equals, info.SpaceName)
}

func (s *SubnetSuite) TestAddSubnetFailsWithEmptyCIDR(c *gc.C) {
	subnetInfo := state.SubnetInfo{}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "missing CIDR")
}

func (s *SubnetSuite) assertAddSubnetForInfoFailsWithSuffix(c *gc.C, subnetInfo state.SubnetInfo, errorSuffix string) error {
	subnet, err := s.State.AddSubnet(subnetInfo)
	errorMessage := fmt.Sprintf("adding subnet %q: %s", subnetInfo.CIDR, errorSuffix)
	c.Assert(err, gc.ErrorMatches, errorMessage)
	c.Assert(subnet, gc.IsNil)
	return err
}

func (s *SubnetSuite) TestAddSubnetFailsWithInvalidCIDR(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "foobar"}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid CIDR address: foobar")
}

func (s *SubnetSuite) TestAddSubnetFailsWithOutOfRangeVLANTag(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.0.1/24", VLANTag: 4095}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid VLAN tag 4095: must be between 0 and 4094")
}

func (s *SubnetSuite) TestAddSubnetFailsWhenAllocatableIPHighSetButAllocatableIPLowNotSet(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.0.1/24", AllocatableIPHigh: "192.168.0.1"}
	s.assertAddSubnetForInfoFailsWithSuffix(
		c, subnetInfo,
		"either both AllocatableIPLow and AllocatableIPHigh must be set or neither set",
	)
}

func (s *SubnetSuite) TestAddSubnetFailsWhenAllocatableIPLowSetButAllocatableIPHighNotSet(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.0.1/24", AllocatableIPLow: "192.168.0.1"}
	s.assertAddSubnetForInfoFailsWithSuffix(
		c, subnetInfo,
		"either both AllocatableIPLow and AllocatableIPHigh must be set or neither set",
	)
}

func (s *SubnetSuite) TestAddSubnetFailsWithInvalidAllocatableIPHigh(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.0.1/24",
		AllocatableIPLow:  "192.168.0.1",
		AllocatableIPHigh: "foobar",
	}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `invalid AllocatableIPHigh "foobar"`)
}

func (s *SubnetSuite) TestAddSubnetFailsWithInvalidAllocatableIPLow(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.0.1/24",
		AllocatableIPLow:  "foobar",
		AllocatableIPHigh: "192.168.0.1",
	}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `invalid AllocatableIPLow "foobar"`)
}

func (s *SubnetSuite) TestAddSubnetFailsWithOutOfRangeAllocatableIPHigh(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.0.1/24",
		AllocatableIPLow:  "192.168.0.1",
		AllocatableIPHigh: "172.168.1.0",
	}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `invalid AllocatableIPHigh "172.168.1.0"`)
}

func (s *SubnetSuite) TestAddSubnetFailsWithOutOfRangeAllocatableIPLow(c *gc.C) {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.0.1/24",
		AllocatableIPLow:  "172.168.1.0",
		AllocatableIPHigh: "192.168.0.10",
	}
	s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `invalid AllocatableIPLow "172.168.1.0"`)
}

func (s *SubnetSuite) TestAddSubnetFailsWithAlreadyExistsForDuplicateCIDRInSameModel(c *gc.C) {
	subnetInfo := state.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	err = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `subnet "192.168.0.1/24" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDuplicateCIDRInDifferentModels(c *gc.C) {
	subnetInfo1 := state.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnetInfo2 := state.SubnetInfo{CIDR: "10.0.0.0/24"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c *gc.C, info1, info2 state.SubnetInfo, otherState *state.State) (*state.Subnet, *state.Subnet) {
	subnet1, err := otherState.AddSubnet(info1)
	c.Assert(err, jc.ErrorIsNil)
	subnet2, err := s.State.AddSubnet(info2)
	c.Assert(err, jc.ErrorIsNil)

	return subnet1, subnet2
}

func (s *SubnetSuite) TestAddSubnetFailsWhenProviderIdNotUniqueInSameModel(c *gc.C) {
	subnetInfo1 := state.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := state.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}

	s.addTwoSubnetsAndAssertSecondFailsWithSuffix(c, subnetInfo1, subnetInfo2, `ProviderId "foo" not unique`)
}

func (s *SubnetSuite) addTwoSubnetsAndAssertSecondFailsWithSuffix(c *gc.C, info1, info2 state.SubnetInfo, errorSuffix string) {
	s.addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c, info1, info2, s.State, errorSuffix)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c *gc.C, info1, info2 state.SubnetInfo, otherState *state.State, errorSuffix string) {
	_, err := otherState.AddSubnet(info1)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddSubnetForInfoFailsWithSuffix(c, info2, errorSuffix)
}

func (s *SubnetSuite) TestAddSubnetSucceedsWhenProviderIdNotUniqueInDifferentModels(c *gc.C) {
	subnetInfo1 := state.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := state.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInSameModel(c *gc.C) {
	subnetInfo1 := state.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := state.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}

	subnet1, subnet2 := s.addTwoSubnetsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsAssertSuccessAndReturnBoth(c *gc.C, info1, info2 state.SubnetInfo) (*state.Subnet, *state.Subnet) {
	return s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, info1, info2, s.State)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInDifferentModels(c *gc.C) {
	subnetInfo1 := state.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := state.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) TestEnsureDeadSetsLifeToDeadWhenAlive(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.refreshAndAssertSubnetLifeIs(c, subnet, state.Dead)
}

func (s *SubnetSuite) addAliveSubnet(c *gc.C, cidr string) *state.Subnet {
	subnetInfo := state.SubnetInfo{CIDR: cidr}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, state.Alive)

	return subnet
}

func (s *SubnetSuite) ensureDeadAndAssertLifeIsDead(c *gc.C, subnet *state.Subnet) {
	err := subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, state.Dead)
}

func (s *SubnetSuite) refreshAndAssertSubnetLifeIs(c *gc.C, subnet *state.Subnet, expectedLife state.Life) {
	err := subnet.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.Life(), gc.Equals, expectedLife)
}

func (s *SubnetSuite) TestEnsureDeadSetsLifeToDeadWhenNotAlive(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
}

func (s *SubnetSuite) TestRemoveFailsIfStillAlive(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")

	err := subnet.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove subnet "192.168.0.1/24": subnet is not dead`)
	s.refreshAndAssertSubnetLifeIs(c, subnet, state.Alive)
}

func (s *SubnetSuite) TestRemoveSucceedsWhenSubnetIsNotAlive(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)

	s.removeSubnetAndAssertNotFound(c, subnet)
}

func (s *SubnetSuite) removeSubnetAndAssertNotFound(c *gc.C, subnet *state.Subnet) {
	err := subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetWithCIDRNotFound(c, subnet.CIDR())
}

func (s *SubnetSuite) assertSubnetWithCIDRNotFound(c *gc.C, cidr string) {
	_, err := s.State.Subnet(cidr)
	s.assertSubnetNotFoundError(c, err)
}

func (s *SubnetSuite) assertSubnetNotFoundError(c *gc.C, err error) {
	c.Assert(err, gc.ErrorMatches, "subnet .* not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SubnetSuite) TestRemoveSucceedsWhenCalledTwice(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.removeSubnetAndAssertNotFound(c, subnet)

	err := subnet.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove subnet "192.168.0.1/24": not found or not dead`)
}

func (s *SubnetSuite) TestRemoveKillsAddedIPAddresses(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.1.0/24")
	s.addIPAddressForSubnet(c, "192.168.1.0", subnet)
	s.addIPAddressForSubnet(c, "192.168.1.1", subnet)

	err := subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertIPAddressNotFound(c, "192.168.1.0")
	s.assertIPAddressNotFound(c, "192.168.1.1")
}

func (s *SubnetSuite) addIPAddressForSubnet(c *gc.C, ipAddress string, subnet *state.Subnet) {
	_, err := s.State.AddIPAddress(network.NewAddress(ipAddress), subnet.ID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SubnetSuite) assertIPAddressNotFound(c *gc.C, ipAddress string) {
	_, err := s.State.IPAddress(ipAddress)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SubnetSuite) TestRefreshUpdatesStaleDocData(c *gc.C) {
	subnet := s.addAliveSubnet(c, "fc00::/64")
	subnetCopy, err := s.State.Subnet("fc00::/64")
	c.Assert(err, jc.ErrorIsNil)

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	c.Assert(subnetCopy.Life(), gc.Equals, state.Alive)

	err = subnetCopy.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetCopy.Life(), gc.Equals, state.Dead)
}

func (s *SubnetSuite) TestRefreshFailsWithNotFoundWhenRemoved(c *gc.C) {
	subnet := s.addAliveSubnet(c, "192.168.1.0/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.removeSubnetAndAssertNotFound(c, subnet)

	err := subnet.Refresh()
	s.assertSubnetNotFoundError(c, err)
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
	subnet := s.addAliveSubnet(c, "192.168.1.0/24")

	_, err := subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches, "no allocatable IP addresses for subnet .*")
}

func (s *SubnetSuite) TestPickNewAddressWhenSubnetIsDead(c *gc.C) {
	subnet := s.addSubnetWithAllocatableIPHigh(c, "192.168.1.0")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)

	_, err := subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches,
		`cannot pick address: subnet "192.168.1.0/24" is not alive`,
	)
}

func (s *SubnetSuite) addSubnetWithAllocatableIPHigh(c *gc.C, allocatableHigh string) *state.Subnet {
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.1.0/24",
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: allocatableHigh,
	}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	return subnet
}

func (s *SubnetSuite) TestPickNewAddressAddressesExhausted(c *gc.C) {
	subnet := s.addSubnetWithAllocatableIPHigh(c, "192.168.1.0")
	s.addIPAddressForSubnet(c, "192.168.1.0", subnet)

	_, err := subnet.PickNewAddress()
	c.Assert(err, gc.ErrorMatches, "allocatable IP addresses exhausted for subnet .*")
}

func (s *SubnetSuite) TestPickNewAddressOneAddress(c *gc.C) {
	subnet := s.addSubnetWithAllocatableIPHigh(c, "192.168.1.0")

	addr, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value(), gc.Equals, "192.168.1.0")
}

func (s *SubnetSuite) TestPickNewAddressSkipsAllocated(c *gc.C) {
	subnet := s.addSubnetWithAllocatableIPHigh(c, "192.168.1.1")
	s.addIPAddressForSubnet(c, "192.168.1.0", subnet)

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
	subnet := s.addSubnetWithAllocatableIPHigh(c, "192.168.1.1")

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
