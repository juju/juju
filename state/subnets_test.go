// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type SubnetSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SubnetSuite{})

func (s *SubnetSuite) TestAddSubnetSucceedsWithFullyPopulatedInfo(c *gc.C) {
	space, err := s.State.AddSpace("foo", "4", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	fanOverlaySubnetInfo := network.SubnetInfo{
		ProviderId: "foo2",
		CIDR:       "10.0.0.0/8",
		SpaceID:    space.Id(),
	}
	subnet, err := s.State.AddSubnet(fanOverlaySubnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, fanOverlaySubnetInfo)
	subnetInfo := network.SubnetInfo{
		ProviderId:        "foo",
		CIDR:              "192.168.1.0/24",
		VLANTag:           79,
		AvailabilityZones: []string{"Timbuktu"},
		SpaceID:           space.Id(),
		ProviderNetworkId: "wildbirds",
		IsPublic:          true,
	}
	subnetInfo.SetFan("10.0.0.0/8", "172.16.0.0/16")

	subnet, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	// check it's been stored in state by fetching it back again
	subnetFromDB, err := s.State.SubnetByCIDR("192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnetFromDB, subnetInfo)
}

func (s *SubnetSuite) assertSubnetMatchesInfo(c *gc.C, subnet *state.Subnet, info network.SubnetInfo) {
	c.Assert(subnet.ProviderId(), gc.Equals, info.ProviderId)
	c.Assert(subnet.CIDR(), gc.Equals, info.CIDR)
	c.Assert(subnet.VLANTag(), gc.Equals, info.VLANTag)
	c.Assert(subnet.AvailabilityZones(), gc.DeepEquals, info.AvailabilityZones)
	c.Assert(subnet.String(), gc.Equals, info.CIDR)
	c.Assert(subnet.GoString(), gc.Equals, info.CIDR)
	expectedSubnetID := info.SpaceID
	if expectedSubnetID == "" {
		expectedSubnetID = "0"
	}
	c.Check(subnet.SpaceID(), gc.Equals, expectedSubnetID)
	c.Assert(subnet.ProviderNetworkId(), gc.Equals, info.ProviderNetworkId)
	c.Assert(subnet.FanLocalUnderlay(), gc.Equals, info.FanLocalUnderlay())
	c.Assert(subnet.FanOverlay(), gc.Equals, info.FanOverlay())
	c.Assert(subnet.IsPublic(), gc.Equals, info.IsPublic)
}

func (s *SubnetSuite) TestAddSubnetFailsWithEmptyCIDR(c *gc.C) {
	subnetInfo := network.SubnetInfo{}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "missing CIDR")
}

func (s *SubnetSuite) assertAddSubnetForInfoFailsWithSuffix(c *gc.C, subnetInfo network.SubnetInfo, errorSuffix string) error {
	subnet, err := s.State.AddSubnet(subnetInfo)
	errorMessage := fmt.Sprintf("adding subnet %q: %s", subnetInfo.CIDR, errorSuffix)
	c.Assert(err, gc.ErrorMatches, errorMessage)
	c.Assert(subnet, gc.IsNil)
	return err
}

func (s *SubnetSuite) TestAddSubnetFailsWithInvalidCIDR(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "foobar"}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid CIDR address: foobar")
}

func (s *SubnetSuite) TestAddSubnetFailsWithOutOfRangeVLANTag(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24", VLANTag: 4095}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid VLAN tag 4095: must be between 0 and 4094")
}

func (s *SubnetSuite) TestAddSubnetFailsWithAlreadyExistsForDuplicateCIDRInSameModel(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	err = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `subnet "192.168.0.1/24" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SubnetSuite) TestAddSubnetSuccessForDuplicateCIDRDiffProviderIDInSameModel(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	subnetInfo.ProviderId = "testme"
	subnet2, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.ID(), gc.Not(gc.Equals), subnet2.ID())
	c.Assert(subnet.CIDR(), gc.Equals, subnet2.CIDR())
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDuplicateCIDRInDifferentModels(c *gc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c *gc.C, info1, info2 network.SubnetInfo, otherState *state.State) (*state.Subnet, *state.Subnet) {
	subnet1, err := otherState.AddSubnet(info1)
	c.Assert(err, jc.ErrorIsNil)
	subnet2, err := s.State.AddSubnet(info2)
	c.Assert(err, jc.ErrorIsNil)

	return subnet1, subnet2
}

func (s *SubnetSuite) TestAddSubnetFailsWhenProviderIdNotUniqueInSameModel(c *gc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}

	s.addTwoSubnetsAndAssertSecondFailsWithSuffix(c, subnetInfo1, subnetInfo2, `provider ID "foo" not unique`)
}

func (s *SubnetSuite) addTwoSubnetsAndAssertSecondFailsWithSuffix(c *gc.C, info1, info2 network.SubnetInfo, errorSuffix string) {
	s.addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c, info1, info2, s.State, errorSuffix)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c *gc.C, info1, info2 network.SubnetInfo, otherState *state.State, errorSuffix string) {
	_, err := otherState.AddSubnet(info1)
	c.Assert(err, jc.ErrorIsNil)

	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, info2, errorSuffix)
}

func (s *SubnetSuite) TestAddSubnetSucceedsWhenProviderIdNotUniqueInDifferentModels(c *gc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInSameModel(c *gc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}

	subnet1, subnet2 := s.addTwoSubnetsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsAssertSuccessAndReturnBoth(c *gc.C, info1, info2 network.SubnetInfo) (*state.Subnet, *state.Subnet) {
	return s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, info1, info2, s.State)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInDifferentModels(c *gc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}
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
	subnetInfo := network.SubnetInfo{CIDR: cidr}
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
	_, err := s.State.SubnetByCIDR(cidr)
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

func (s *SubnetSuite) TestRefreshUpdatesStaleDocData(c *gc.C) {
	subnet := s.addAliveSubnet(c, "fc00::/64")
	subnetCopy, err := s.State.SubnetByCIDR("fc00::/64")
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
	space1, err := s.State.AddSpace("bar", "4", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	space2, err := s.State.AddSpace("notreally", "5", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	subnetInfos := []network.SubnetInfo{
		{CIDR: "192.168.1.0/24"},
		{CIDR: "8.8.8.0/24", SpaceID: space1.Id()},
		{CIDR: "10.0.2.0/24", ProviderId: "foo"},
		{CIDR: "2001:db8::/64", AvailabilityZones: []string{"zone1"}},
		{CIDR: "253.0.0.0/8", SpaceID: space2.Id()},
	}
	subnetInfos[4].SetFan("8.8.8.0/24", "")

	for _, info := range subnetInfos {
		_, err := s.State.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
	}

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, len(subnetInfos))

	for i, subnet := range subnets {
		c.Check(subnet.CIDR(), gc.Equals, subnetInfos[i].CIDR)
		c.Check(subnet.ProviderId(), gc.Equals, subnetInfos[i].ProviderId)
		if subnet.FanLocalUnderlay() == "" {
			expectedSubnetID := subnetInfos[i].SpaceID
			if expectedSubnetID == "" {
				expectedSubnetID = "0"
			}
			c.Check(subnet.SpaceID(), gc.Equals, expectedSubnetID)
		} else {
			// Special case
			c.Check(subnet.SpaceID(), gc.Equals, space1.Id())
		}
		c.Check(subnet.AvailabilityZones(), gc.DeepEquals, subnetInfos[i].AvailabilityZones)
	}
}

func (s *SubnetSuite) TestUpdateMAASUndefinedSpace(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "8.8.8.0/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace(names.NewSpaceTag("undefined").Id(), "-1", []string{subnet.ID()}, false)
	c.Assert(err, jc.ErrorIsNil)

	subnetInfo.SpaceName = "testme"
	_, err = s.State.AddSpace(names.NewSpaceTag(subnetInfo.SpaceName).Id(), "2", []string{}, false)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Update(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.SpaceName(), gc.Equals, subnetInfo.SpaceName)
}

func (s *SubnetSuite) TestUpdateEmpty(c *gc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "8.8.8.0/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	subnetInfo.VLANTag = 76
	subnetInfo.AvailabilityZones = []string{"testme-az"}
	subnetInfo.SpaceName = "testme"
	_, err = s.State.AddSpace(names.NewSpaceTag(subnetInfo.SpaceName).Id(), "2", []string{}, false)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Update(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.VLANTag(), gc.Equals, subnetInfo.VLANTag)
	c.Assert(subnet.AvailabilityZones(), gc.DeepEquals, subnetInfo.AvailabilityZones)
	c.Assert(subnet.SpaceName(), gc.Equals, subnetInfo.SpaceName)
}

func (s *SubnetSuite) TestUpdateNonEmpty(c *gc.C) {
	expectedSubnetInfo := network.SubnetInfo{
		CIDR: "8.8.8.0/24", VLANTag: 42, AvailabilityZones: []string{"changeme-az", "testme-az"}}
	subnet, err := s.State.AddSubnet(expectedSubnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	expectedSpace, err := s.State.AddSpace("changeme", "2", []string{subnet.ID()}, false)
	c.Assert(err, jc.ErrorIsNil)

	newSubnetInfo := network.SubnetInfo{
		CIDR:              subnet.CIDR(),
		SpaceName:         "testme",
		VLANTag:           76,
		AvailabilityZones: []string{"testme-az"},
	}
	_, err = s.State.AddSpace(names.NewSpaceTag(newSubnetInfo.SpaceName).Id(), "7", []string{}, false)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Update(newSubnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.SpaceID(), gc.Equals, expectedSpace.Id())
	c.Assert(subnet.VLANTag(), gc.Equals, expectedSubnetInfo.VLANTag)
	c.Assert(subnet.AvailabilityZones(), gc.DeepEquals, expectedSubnetInfo.AvailabilityZones)
}

func (s *SubnetSuite) TestUniqueAdditionAndRetrievalByCIDR(c *gc.C) {
	cidr := "1.1.1.1/24"

	sub1 := network.SubnetInfo{
		CIDR:              cidr,
		ProviderId:        "1",
		ProviderNetworkId: "1",
	}
	_, err := s.State.AddSubnet(sub1)
	c.Assert(err, jc.ErrorIsNil)

	sub2 := network.SubnetInfo{
		CIDR:              cidr,
		ProviderId:        "2",
		ProviderNetworkId: "2",
	}
	_, err = s.State.AddSubnet(sub2)
	c.Assert(err, jc.ErrorIsNil)

	subs, err := s.State.SubnetsByCIDR(cidr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subs, gc.HasLen, 2)

	_, err = s.State.SubnetByCIDR(cidr)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("multiple subnets matching %q", cidr))
}

func (s *SubnetSuite) TestUpdateSpaceOps(c *gc.C) {
	space, err := s.State.AddSpace("space-0", "0", []string{}, false)
	c.Assert(err, jc.ErrorIsNil)

	arg := network.SubnetInfo{
		CIDR:              "10.10.10.0/24",
		ProviderId:        "1",
		ProviderNetworkId: "1",
		SpaceID:           space.Id(),
	}

	sub, err := s.State.AddSubnet(arg)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(sub.UpdateSpaceOps(space.Id()), gc.IsNil)

	ops := sub.UpdateSpaceOps("666")
	c.Assert(ops, gc.HasLen, 2)
	for _, op := range ops {
		if op.C == "spaces" {
			c.Check(op.Id, gc.Equals, fmt.Sprintf("%s:666", s.State.ModelUUID()))
			c.Check(op.Assert, gc.DeepEquals, txn.DocExists)
		} else if op.C == "subnets" {
			c.Check(op.Id, gc.Equals, fmt.Sprintf("%s:%s", s.State.ModelUUID(), sub.ID()))
			c.Check(op.Update, gc.DeepEquals, bson.D{{"$set", bson.D{{"space-id", "666"}}}})
			c.Check(op.Assert, gc.DeepEquals, bson.D{{"life", state.Alive}})
		} else {
			c.Fatalf("unexpected txn.Op collection: %q", op.C)
		}
	}
}
