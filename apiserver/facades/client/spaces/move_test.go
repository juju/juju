// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

type moveSubnetsAPISuite struct {
	spaces.APISuite
}

var _ = gc.Suite(&moveSubnetsAPISuite{})

func (s *moveSubnetsAPISuite) TestMoveSubnetsSubnetNotFoundError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"

	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(nil, errors.NotFoundf("subnet 3"))

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "subnet 3 not found")
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsUnaffectedSubnetSuccess(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "666",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), spaceName).Return(&allSpaces[1], nil)
	s.SubnetService.EXPECT().UpdateSubnet(gomock.Any(), subnetID, "2").Return(nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return(nil, nil)
	bExp.AllEndpointBindings(allSpaces).Return(nil, nil)

	// Using different subnet - triggers no constraint violation.
	m := expectMachine(ctrl, "20.20.20.0/24")
	expectMachineUnits(ctrl, m, "mysql", "mysql/0")
	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m}, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.DeepEquals, []params.MoveSubnetsResult{{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "subnet-3",
			OldSpaceTag: "space-from",
			CIDR:        cidr,
		}},
		NewSpaceTag: "space-destination",
		Error:       nil,
	}})
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsNoSpaceConstraintsSuccess(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "666",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), spaceName).Return(&allSpaces[1], nil)
	s.SubnetService.EXPECT().UpdateSubnet(gomock.Any(), subnetID, "2").Return(nil)

	// MySQL has only non-space constraints.
	cons1 := spaces.NewMockConstraints(ctrl)
	cons1.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons1.EXPECT().Value().Return(constraints.MustParse("arch=amd64"))

	// Some other unaffected application is constrained not to be in the space.
	cons2 := spaces.NewMockConstraints(ctrl)
	cons2.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#wordpress")
	cons2.EXPECT().Value().Return(constraints.MustParse("spaces=^destination"))

	m := expectMachine(ctrl, cidr)
	expectMachineUnits(ctrl, m, "mysql", "mysql/0")
	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons1, cons2}, nil)
	bExp.AllEndpointBindings(allSpaces).Return(nil, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsNegativeConstraintsViolatedNoForceError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "666",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)

	// MySQL is constrained not to be in the target space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=^destination"))

	m := expectMachine(ctrl, cidr)
	expectMachineUnits(ctrl, m, "mysql", "mysql/0")
	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet(s) to space "destination" violates space constraints for application "mysql": ^destination`)
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsNegativeConstraintsViolatedForOverlayNoForceError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"
	fanSubnetID := "666"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	// The network topology indicates that the moving subnet has a fan overlay,
	// which will also move the the new space implicitly.
	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "old-space",
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				// This simulates what we see in AWS, where the overlay is
				// segmented based on zones.
				// See below where we create an address in this Fan network.
				{
					ID:   network.Id(fanSubnetID),
					CIDR: "10.10.0.0/12",
					FanInfo: &network.FanCIDRs{
						FanLocalUnderlay: cidr,
						FanOverlay:       "10.0.0.0/8",
					},
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)

	// MySQL is constrained not to be in the target space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=^destination"))

	// This address is reported as being in the main Fan overlay;
	// not the segment in our network topology.
	// So we expect the subnet to be looked up by the address value.
	address := spaces.NewMockAddress(ctrl)
	address.EXPECT().SubnetCIDR().Return("10.0.0.0/8")
	address.EXPECT().ConfigMethod().Return(network.ConfigDHCP)
	address.EXPECT().Value().Return("10.10.0.5")

	m := spaces.NewMockMachine(ctrl)
	m.EXPECT().AllAddresses().Return([]spaces.Address{address}, nil)

	expectMachineUnits(ctrl, m, "mysql", "mysql/0")
	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet(s) to space "destination" violates space constraints for application "mysql": ^destination`)
}

func (s *moveSubnetsAPISuite) TestSubnetsNegativeConstraintsViolatedForceSuccess(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "666",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), spaceName).Return(&allSpaces[1], nil)
	s.SubnetService.EXPECT().UpdateSubnet(gomock.Any(), subnetID, "2").Return(nil)

	// MySQL is constrained not to be in the target space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=^destination"))

	m := expectMachine(ctrl, cidr)
	expectMachineUnits(ctrl, m, "mysql", "mysql/0")
	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)
	bExp.AllEndpointBindings(allSpaces).Return(nil, nil)

	// Supplying force=true succeeds despite the violation.
	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, true))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.DeepEquals, []params.MoveSubnetsResult{{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "subnet-3",
			OldSpaceTag: "space-from",
			CIDR:        cidr,
		}},
		NewSpaceTag: "space-destination",
		Error:       nil,
	}})
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsPositiveConstraintsViolatedNoForceError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			// Note the two subnets in the original space.
			// We are only moving one.
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "6",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)

	// MySQL is constrained to be in a different space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=from"))

	// mysql/0 is connected to both the moving subnet and the stationary one.
	// It will satisfy the constraint even after the subnet relocation.
	m1 := expectMachine(ctrl, cidr, "20.20.20.0/24")
	expectMachineUnits(ctrl, m1, "mysql", "mysql/0")

	// This machine's units are connected only to the moving subnet,
	// which will violate the constraint.
	m2 := expectMachine(ctrl, cidr)
	expectMachineUnits(ctrl, m2, "mysql", "mysql/1", "mysql/2")

	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m1, m2}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet(s) to space "destination" violates space constraints for application "mysql": from
	units not connected to the space: mysql/1, mysql/2`)
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsEndpointBindingsViolatedNoForceError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.10.10.0/24"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      cidr,
		SpaceName: "from",
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	allSpaces := network.SpaceInfos{
		{
			ID:   "1",
			Name: "from",
			// Note the two subnets in the original space.
			// We are only moving one.
			Subnets: network.SubnetInfos{
				{
					ID:   network.Id(subnetID),
					CIDR: cidr,
				},
				{
					ID:   "6",
					CIDR: "20.20.20.0/24",
				},
			},
		},
		{
			ID:   "2",
			Name: network.SpaceName(spaceName),
		},
	}
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(allSpaces, nil)

	// MySQL has a binding to the old space.
	bindings := spaces.NewMockBindings(ctrl)
	bindings.EXPECT().Map().Return(map[string]string{"db": "1"})

	// mysql/0 is connected to both the moving subnet and the stationary one.
	// It will satisfy the binding even after the subnet relocation.
	m1 := expectMachine(ctrl, cidr, "20.20.20.0/24")
	expectMachineUnits(ctrl, m1, "mysql", "mysql/0")

	// This machine's units are connected only to the moving subnet,
	// which will violate the binding.
	m2 := expectMachine(ctrl, cidr)
	expectMachineUnits(ctrl, m2, "mysql", "mysql/1", "mysql/2")

	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{m1, m2}, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return(nil, nil)
	bExp.AllEndpointBindings(allSpaces).Return(map[string]spaces.Bindings{"mysql": bindings}, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet(s) to space "destination" violates endpoint binding db:from for application "mysql"
	units not connected to the space: mysql/1, mysql/2`)
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsHasUnderlayError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"

	subnet := &network.SubnetInfo{
		ID:        network.Id(subnetID),
		CIDR:      "10.0.0.0/8",
		SpaceName: "from",
		FanInfo: &network.FanCIDRs{
			FanLocalUnderlay: "20.0.0.0/24",
		},
	}
	s.SubnetService.EXPECT().Subnet(gomock.Any(), subnetID).Return(subnet, nil)

	res, err := s.API.MoveSubnets(context.Background(), moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.NotNil)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`subnet "10.0.0.0/8" is a fan overlay of "20.0.0.0/24" and cannot be moved; move the underlay instead`)
}

func expectMachine(ctrl *gomock.Controller, cidrs ...string) *spaces.MockMachine {
	addrs := make([]spaces.Address, len(cidrs))
	for i, cidr := range cidrs {
		address := spaces.NewMockAddress(ctrl)
		address.EXPECT().SubnetCIDR().Return(cidr)
		address.EXPECT().ConfigMethod().Return(network.ConfigDHCP)
		addrs[i] = address
	}

	// Add a loopback into the mix to test that we don't ask for its subnets.
	loopback := spaces.NewMockAddress(ctrl)
	loopback.EXPECT().ConfigMethod().Return(network.ConfigLoopback)

	machine := spaces.NewMockMachine(ctrl)
	machine.EXPECT().AllAddresses().Return(append(addrs, loopback), nil)
	return machine
}

func expectMachineUnits(ctrl *gomock.Controller, machine *spaces.MockMachine, appName string, unitNames ...string) {
	units := make([]spaces.Unit, len(unitNames))
	for i, unitName := range unitNames {
		unit := spaces.NewMockUnit(ctrl)
		unit.EXPECT().Name().Return(unitName)
		unit.EXPECT().ApplicationName().Return(appName)
		units[i] = unit
	}

	machine.EXPECT().Units().Return(units, nil)
}

func moveSubnetsArg(subnetID, spaceName string, force bool) params.MoveSubnetsParams {
	return params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SubnetTags: []string{names.NewSubnetTag(subnetID).String()},
			SpaceTag:   names.NewSpaceTag(spaceName).String(),
			Force:      force,
		}},
	}
}
