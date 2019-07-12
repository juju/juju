// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type SpacesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) addSubnets(c *gc.C, CIDRs []string) {
	s.addSubnetsForState(c, CIDRs, s.State)
}

func (s *SpacesSuite) addSubnetsForState(c *gc.C, CIDRs []string, st *state.State) {
	if len(CIDRs) == 0 {
		return
	}
	for _, info := range s.makeSubnetInfosForCIDRs(c, CIDRs) {
		_, err := st.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *SpacesSuite) makeSubnetInfosForCIDRs(c *gc.C, CIDRs []string) []state.SubnetInfo {
	infos := make([]state.SubnetInfo, len(CIDRs))
	for i, cidr := range CIDRs {
		_, _, err := net.ParseCIDR(cidr)
		c.Assert(err, jc.ErrorIsNil)

		infos[i] = state.SubnetInfo{
			CIDR:             cidr,
			VLANTag:          79,
			AvailabilityZone: "AvailabilityZone",
		}

	}
	return infos
}

type addSpaceArgs struct {
	Name        string
	ProviderId  network.Id
	SubnetCIDRs []string
	IsPublic    bool
	ForState    *state.State
}

func (s *SpacesSuite) addSpaceWithSubnets(c *gc.C, args addSpaceArgs) (*state.Space, error) {
	if args.ForState == nil {
		args.ForState = s.State
	}
	s.addSubnetsForState(c, args.SubnetCIDRs, args.ForState)
	return args.ForState.AddSpace(args.Name, args.ProviderId, args.SubnetCIDRs, args.IsPublic)
}

func (s *SpacesSuite) assertSpaceNotFound(c *gc.C, name string) {
	s.assertSpaceNotFoundForState(c, name, s.State)
}

func (s *SpacesSuite) assertSpaceNotFoundForState(c *gc.C, name string, st *state.State) {
	_, err := st.Space(name)
	s.assertSpaceNotFoundError(c, err, name)
}

func (s *SpacesSuite) assertSpaceNotFoundError(c *gc.C, err error, name string) {
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("space %q not found", name))

	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SpacesSuite) assertSpaceMatchesArgs(c *gc.C, space *state.Space, args addSpaceArgs) {
	c.Assert(space.Name(), gc.Equals, args.Name)
	c.Assert(space.ProviderId(), gc.Equals, args.ProviderId)

	actualSubnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	actualSubnetIds := make([]string, len(actualSubnets))
	for i, subnet := range actualSubnets {
		actualSubnetIds[i] = subnet.CIDR()
	}
	c.Assert(actualSubnetIds, jc.SameContents, args.SubnetCIDRs)
	c.Assert(state.SpaceDoc(space).IsPublic, gc.Equals, args.IsPublic)

	c.Assert(space.Life(), gc.Equals, state.Alive)
	c.Assert(space.String(), gc.Equals, args.Name)

	// The space ID is not empty and not equivalent to the default space.
	c.Assert(space.Id(), gc.Not(gc.Equals), "")
	c.Assert(space.Id(), gc.Not(gc.Equals), "0")
}

func (s *SpacesSuite) TestAddSpaceWithNoSubnetsAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: nil,
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)

}

func (s *SpacesSuite) TestAddSpaceWithNoSubnetsAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("my provider id"),
		SubnetCIDRs: nil,
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv4SubnetAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"1.1.1.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv4SubnetAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("some id"),
		SubnetCIDRs: []string{"1.1.1.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv6SubnetAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"fc00:123::/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv6SubnetAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("provider id"),
		SubnetCIDRs: []string{"fc00:123::/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv4AndOneIPv6SubnetAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"1.1.1.0/24", "fc00::123/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithOneIPv4AndOneIPv6SubnetAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("foo bar"),
		SubnetCIDRs: []string{"1.1.1.0/24", "fc00::123/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv4SubnetsAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"1.1.1.0/24", "1.2.2.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv4SubnetsAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("My Provider ID"),
		SubnetCIDRs: []string{"1.1.1.0/24", "1.2.2.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv6SubnetsAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"fc00:123::/64", "fc00:321::/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv6SubnetsAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("My Provider ID"),
		SubnetCIDRs: []string{"fc00:123::/64", "fc00:321::/64"},
	}
	space, err := s.addSpaceWithSubnets(c, args)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv4AndIPv6SubnetsAndEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  "",
		SubnetCIDRs: []string{"fc00:123::/64", "2.2.2.0/20", "fc00:321::/64", "1.1.1.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)

}

func (s *SpacesSuite) TestAddSpaceWithMultipleIPv4AndIPv6SubnetsAndNonEmptyProviderId(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		ProviderId:  network.Id("My Provider ID"),
		SubnetCIDRs: []string{"fc00:123::/64", "2.2.2.0/20", "fc00:321::/64", "1.1.1.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
}

func (s *SpacesSuite) addTwoSpacesReturnSecond(c *gc.C, args1, args2 addSpaceArgs) (*state.Space, error) {
	space1, err := s.addSpaceWithSubnets(c, args1)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space1, args1)

	return s.addSpaceWithSubnets(c, args2)
}

func (s *SpacesSuite) TestAddTwoSpacesWithDifferentNamesButSameProviderIdFailsInSameModel(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: network.Id("provider id"),
	}
	args2 := args1
	args2.Name = "different"

	_, err := s.addTwoSpacesReturnSecond(c, args1, args2)
	s.assertProviderIdNotUniqueErrorForArgs(c, err, args2)
}

func (s *SpacesSuite) assertProviderIdNotUniqueErrorForArgs(c *gc.C, err error, args addSpaceArgs) {
	expectedError := fmt.Sprintf("adding space %q: provider ID %q not unique", args.Name, args.ProviderId)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *SpacesSuite) TestAddTwoSpacesWithDifferentNamesButSameProviderIdSucceedsInDifferentModels(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: network.Id("provider id"),
		ForState:   s.State,
	}
	args2 := args1
	args2.Name = "different"
	args2.ForState = s.NewStateForModelNamed(c, "other")

	space2, err := s.addTwoSpacesReturnSecond(c, args1, args2)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space2, args2)
}

func (s *SpacesSuite) TestAddTwoSpacesWithDifferentNamesAndEmptyProviderIdSucceedsInSameModel(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: "",
	}
	args2 := args1
	args2.Name = "different"

	space2, err := s.addTwoSpacesReturnSecond(c, args1, args2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space2, args2)
}

func (s *SpacesSuite) TestAddTwoSpacesWithDifferentNamesAndEmptyProviderIdSucceedsInDifferentModels(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: "",
		ForState:   s.State,
	}
	args2 := args1
	args2.Name = "different"
	args2.ForState = s.NewStateForModelNamed(c, "other")

	space2, err := s.addTwoSpacesReturnSecond(c, args1, args2)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space2, args2)
}

func (s *SpacesSuite) TestAddTwoSpacesWithSameNamesAndEmptyProviderIdsFailsInSameModel(c *gc.C) {
	args := addSpaceArgs{
		Name:       "my-space",
		ProviderId: "",
	}

	_, err := s.addTwoSpacesReturnSecond(c, args, args)
	s.assertSpaceAlreadyExistsErrorForArgs(c, err, args)
}

func (s *SpacesSuite) assertSpaceAlreadyExistsErrorForArgs(c *gc.C, err error, args addSpaceArgs) {
	expectedError := fmt.Sprintf("adding space %q: space %q already exists", args.Name, args.Name)
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SpacesSuite) TestAddTwoSpacesWithSameNamesAndProviderIdsFailsInTheSameModel(c *gc.C) {
	args := addSpaceArgs{
		Name:       "my-space",
		ProviderId: network.Id("does not matter if not empty"),
	}

	_, err := s.addTwoSpacesReturnSecond(c, args, args)
	s.assertSpaceAlreadyExistsErrorForArgs(c, err, args)
}

func (s *SpacesSuite) TestAddTwoSpacesWithSameNamesAndEmptyProviderIdsSuccedsInDifferentModels(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: "",
		ForState:   s.State,
	}
	args2 := args1
	args2.ForState = s.NewStateForModelNamed(c, "other")

	space2, err := s.addTwoSpacesReturnSecond(c, args1, args2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space2, args2)
}

func (s *SpacesSuite) TestAddTwoSpacesWithSameNamesAndProviderIdsSuccedsInDifferentModels(c *gc.C) {
	args1 := addSpaceArgs{
		Name:       "my-space",
		ProviderId: network.Id("same provider id"),
		ForState:   s.State,
	}
	args2 := args1
	args2.ForState = s.NewStateForModelNamed(c, "other")

	space2, err := s.addTwoSpacesReturnSecond(c, args1, args2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space2, args2)
}

func (s *SpacesSuite) TestAddSpaceWhenSubnetNotFound(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false

	_, err := s.State.AddSpace(name, "", subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, `adding space "my-space": subnet "1.1.1.0/24" not found`)
	s.assertSpaceNotFound(c, name)
}

func (s *SpacesSuite) TestAddSpaceWithNonEmptyProviderIdAndInvalidNameFails(c *gc.C) {
	args := addSpaceArgs{
		Name:       "-bad name-",
		ProviderId: network.Id("My Provider ID"),
	}
	_, err := s.addSpaceWithSubnets(c, args)
	s.assertInvalidSpaceNameErrorAndWasNotAdded(c, err, args.Name)
}

func (s *SpacesSuite) assertInvalidSpaceNameErrorAndWasNotAdded(c *gc.C, err error, name string) {
	expectedError := fmt.Sprintf("adding space %q: invalid space name", name)
	c.Assert(err, gc.ErrorMatches, expectedError)

	// The default space will be present, although we cannot add it.
	// Only check non-default names.
	if name != environs.DefaultSpaceName {
		s.assertSpaceNotFound(c, name)
	}
}

func (s *SpacesSuite) TestAddSpaceWithEmptyProviderIdAndInvalidNameFails(c *gc.C) {
	args := addSpaceArgs{
		Name:       "-bad name-",
		ProviderId: "",
	}
	_, err := s.addSpaceWithSubnets(c, args)
	s.assertInvalidSpaceNameErrorAndWasNotAdded(c, err, args.Name)
}

func (s *SpacesSuite) TestAddSpaceWithEmptyNameAndProviderIdFails(c *gc.C) {
	args := addSpaceArgs{
		Name:       "",
		ProviderId: "",
	}
	_, err := s.addSpaceWithSubnets(c, args)
	s.assertInvalidSpaceNameErrorAndWasNotAdded(c, err, args.Name)
}

func (s *SpacesSuite) TestAddSpaceWithEmptyNameAndNonEmptyProviderIdFails(c *gc.C) {
	args := addSpaceArgs{
		Name:       "",
		ProviderId: network.Id("doesn't matter"),
	}
	_, err := s.addSpaceWithSubnets(c, args)
	s.assertInvalidSpaceNameErrorAndWasNotAdded(c, err, args.Name)
}

func (s *SpacesSuite) TestSubnetsReturnsExpectedSubnets(c *gc.C) {
	args := addSpaceArgs{
		Name:        "my-space",
		SubnetCIDRs: []string{"1.1.1.0/24", "2.1.1.0/24", "3.1.1.0/24", "4.1.1.0/24", "5.1.1.0/24"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)

	var expected []*state.Subnet
	for _, cidr := range args.SubnetCIDRs {
		subnet, err := s.State.Subnet(cidr)
		c.Assert(err, jc.ErrorIsNil)
		expected = append(expected, subnet)
	}
	actual, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.DeepEquals, expected)
}

func (s *SpacesSuite) TestAllSpaces(c *gc.C) {
	defaultSpace, err := s.State.Space("")
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, jc.DeepEquals, []*state.Space{defaultSpace})

	subnets := []string{"1.1.1.0/24", "2.1.1.0/24", "3.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	first, err := s.State.AddSpace("first", "", []string{"1.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	second, err := s.State.AddSpace("second", "", []string{"2.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	third, err := s.State.AddSpace("third", "", []string{"3.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)

	actual, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.SameContents, []*state.Space{first, second, third, defaultSpace})
}

func (s *SpacesSuite) TestEnsureDeadSetsLifeToDeadWhenAlive(c *gc.C) {
	space := s.addAliveSpace(c, "alive")

	s.ensureDeadAndAssertLifeIsDead(c, space)
	s.refreshAndAssertSpaceLifeIs(c, space, state.Dead)
}

func (s *SpacesSuite) addAliveSpace(c *gc.C, name string) *state.Space {
	space, err := s.State.AddSpace(name, "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Life(), gc.Equals, state.Alive)
	return space
}

func (s *SpacesSuite) ensureDeadAndAssertLifeIsDead(c *gc.C, space *state.Space) {
	err := space.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Life(), gc.Equals, state.Dead)
}

func (s *SpacesSuite) refreshAndAssertSpaceLifeIs(c *gc.C, space *state.Space, expectedLife state.Life) {
	err := space.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Life(), gc.Equals, expectedLife)
}

func (s *SpacesSuite) TestEnsureDeadSetsLifeToDeadWhenNotAlive(c *gc.C) {
	space := s.addAliveSpace(c, "soon-dead")
	s.ensureDeadAndAssertLifeIsDead(c, space)

	s.ensureDeadAndAssertLifeIsDead(c, space)
}

func (s *SpacesSuite) TestRemoveFailsIfStillAlive(c *gc.C) {
	space := s.addAliveSpace(c, "still-alive")

	err := space.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove space "still-alive": space is not dead`)

	s.refreshAndAssertSpaceLifeIs(c, space, state.Alive)
}

func (s *SpacesSuite) TestRemoveSucceedsWhenSpaceIsNotAlive(c *gc.C) {
	space := s.addAliveSpace(c, "not-alive-soon")
	s.ensureDeadAndAssertLifeIsDead(c, space)

	s.removeSpaceAndAssertNotFound(c, space)
}

func (s *SpacesSuite) removeSpaceAndAssertNotFound(c *gc.C, space *state.Space) {
	err := space.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceNotFound(c, space.Name())
}

func (s *SpacesSuite) TestRemoveSucceedsWhenCalledTwice(c *gc.C) {
	space := s.addAliveSpace(c, "twice-deleted")
	s.ensureDeadAndAssertLifeIsDead(c, space)
	s.removeSpaceAndAssertNotFound(c, space)

	err := space.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove space "twice-deleted": not found or not dead`)
}

func (s *SpacesSuite) TestRefreshUpdatesStaleDocData(c *gc.C) {
	space := s.addAliveSpace(c, "original")
	spaceCopy, err := s.State.Space(space.Name())
	c.Assert(err, jc.ErrorIsNil)

	s.ensureDeadAndAssertLifeIsDead(c, space)
	c.Assert(spaceCopy.Life(), gc.Equals, state.Alive)

	err = spaceCopy.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaceCopy.Life(), gc.Equals, state.Dead)
}

func (s *SpacesSuite) TestRefreshFailsWithNotFoundWhenRemoved(c *gc.C) {
	space := s.addAliveSpace(c, "soon-removed")
	s.ensureDeadAndAssertLifeIsDead(c, space)
	s.removeSpaceAndAssertNotFound(c, space)

	err := space.Refresh()
	s.assertSpaceNotFoundError(c, err, "soon-removed")
}

func (s *SpacesSuite) TestFanSubnetInheritsSpace(c *gc.C) {
	args := addSpaceArgs{
		Name:        "space1",
		ProviderId:  network.Id("some id 2"),
		SubnetCIDRs: []string{"1.1.1.0/24", "2001:cbd0::/32"},
	}
	space, err := s.addSpaceWithSubnets(c, args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSpaceMatchesArgs(c, space, args)
	info := state.SubnetInfo{
		CIDR:             "253.1.0.0/16",
		VLANTag:          79,
		AvailabilityZone: "AvailabilityZone",
		FanOverlay:       "253.0.0.0/8",
		FanLocalUnderlay: "1.1.1.0/24",
	}
	_, err = s.State.AddSubnet(info)
	c.Assert(err, jc.ErrorIsNil)

	err = space.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	var foundSubnet *state.Subnet
	for _, subnet := range subnets {
		if subnet.CIDR() == "253.1.0.0/16" {
			foundSubnet = subnet
			break
		}
	}
	c.Assert(foundSubnet, gc.NotNil)
	c.Assert(foundSubnet.SpaceName(), gc.Equals, "space1")
}
