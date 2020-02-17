// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// UpdateSubnet describes a subnet that can be updated.
type UpdateSubnet interface {
	UpdateOps(args network.SubnetInfo) ([]txn.Op, error)
	Refresh() error
	CIDR() string
	SpaceName() string
}

// MovedCDIR holds the movement from a CIDR from space `a` to space `b`
type MovedCDIR struct {
	FromSpace string
	CIDR      string
}

// MoveToSpaceModelOp describes a model operation for moving cidrs to a space
type MoveToSpaceModelOp interface {
	state.ModelOperation

	// GetMovedCIDRs returns the moved cidrs resulting from
	// successfully moving cidrs.
	GetMovedCIDRs() []MovedCDIR
}

type moveToSpaceModelOp struct {
	subnets    []UpdateSubnet
	spaceName  string
	movedCDIRs []MovedCDIR
}

func (o *moveToSpaceModelOp) GetMovedCIDRs() []MovedCDIR {
	return o.movedCDIRs
}

func (o *moveToSpaceModelOp) Done(err error) error {
	return err
}

func NewUpdateSpaceModelOp(spaceName string, subnets []UpdateSubnet) *moveToSpaceModelOp {
	return &moveToSpaceModelOp{
		subnets:   subnets,
		spaceName: spaceName,
	}
}

func (o *moveToSpaceModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		for _, subnet := range o.subnets {
			if err := subnet.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	movedCIDRS := make([]MovedCDIR, len(o.subnets))
	for i, subnet := range o.subnets {
		mc := MovedCDIR{
			FromSpace: subnet.SpaceName(),
			CIDR:      subnet.CIDR(),
		}
		movedCIDRS[i] = mc
	}

	var totalOps []txn.Op
	argToUpdate := network.SubnetInfo{
		SpaceName: o.spaceName,
	}
	for _, subnet := range o.subnets {
		ops, err := subnet.UpdateOps(argToUpdate)
		if err != nil {
			return nil, errors.Trace(err)
		}
		totalOps = append(totalOps, ops...)
	}
	o.movedCDIRs = movedCIDRS
	return totalOps, nil
}

// MoveToSpace updates a space by it's given CIDR
func (api *API) MoveToSpace(args params.MoveToSpaceParams) (params.MoveToSpaceResults, error) {
	var results params.MoveToSpaceResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return results, err
	}

	results = params.MoveToSpaceResults{
		Results: make([]params.MoveToSpaceResult, len(args.MoveToSpace)),
	}

	for i, toSpaceParams := range args.MoveToSpace {
		spaceTag, err := names.ParseSpaceTag(toSpaceParams.SpaceTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		subnets, err := api.getValidSubnetsByCIDR(toSpaceParams.CIDRs)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if err := api.checkSubnetAllowedToBeMoved(subnets, toSpaceParams.Force, spaceTag); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		operation, err := api.opFactory.NewUpdateSpaceModelOp(spaceTag.Id(), subnets)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if err = api.backing.ApplyOperation(operation); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		} else {
			createMovedCidrs(operation.GetMovedCIDRs(), &results.Results[i])
		}

	}
	return results, nil
}

func createMovedCidrs(movedCDIRSpace []MovedCDIR, result *params.MoveToSpaceResult) {
	result.Moved = make([]params.MovedSpaceCIDR, len(movedCDIRSpace))
	for i, v := range movedCDIRSpace {
		result.Moved[i].CIDR = v.CIDR
		result.Moved[i].SpaceTag = names.NewSpaceTag(v.FromSpace).String()
	}
}

func (api *API) checkSubnetAllowedToBeMoved(subnets []networkingcommon.BackingSubnet, force bool, spaceTag names.SpaceTag) error {

	space, err := api.backing.SpaceByName(spaceTag.Id())
	if err != nil {
		return errors.Trace(err)
	}

	if errString := checkSubnetAlreadyInSpace(subnets, space); len(errString) > 0 {
		return errors.Trace(errors.New(strings.Join(errString, "\n")))
	}

	machines, err := api.backing.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	return checkSubnetInUse(subnets, machines, force)
}

func checkSubnetInUse(subnets []networkingcommon.BackingSubnet, machines []Machine, force bool) error {
	subnetCIDRs := set.NewStrings()
	for _, subnet := range subnets {
		subnetCIDRs.Add(subnet.CIDR())
	}

	var errorStrings []string
	for _, machine := range machines {
		addresses, err := machine.AllAddresses()
		if err != nil {
			return errors.Trace(err)
		}
		for _, address := range addresses {
			if subnetCIDRs.Contains(address.SubnetCIDR()) {
				errorStrings = append(errorStrings, fmt.Sprintf("machine %q already has a address on subnet %q", machine.Id(), address.SubnetCIDR()))
			}
		}
	}
	if len(errorStrings) != 0 && !force {
		return errors.Errorf(strings.Join(errorStrings, "\n"))
	}
	return nil
}

func checkSubnetAlreadyInSpace(subnets []networkingcommon.BackingSubnet, space networkingcommon.BackingSpace) []string {
	var errorStrings []string
	for _, subnet := range subnets {
		if subnet.SpaceID() == space.Id() {
			msg := fmt.Sprintf("supplied CIDR %q is already in space %q", subnet.CIDR(), space.Name())
			errorStrings = append(errorStrings, msg)
		}
	}
	return errorStrings
}
