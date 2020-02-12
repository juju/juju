//Copyright 2020 Canonical Ltd.
//Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
)

// UpdateSubnet describes a subnet that can be updated.
type UpdateSubnet interface {
	UpdateOps(args network.SubnetInfo) ([]txn.Op, error)
	Refresh() error
}

type spaceUpdateModelOp struct {
	subnets []UpdateSubnet
	spaceID string
}

func (o *spaceUpdateModelOp) Done(err error) error {
	return err
}

func NewUpdateSpaceModelOp(spaceID string, subnets []UpdateSubnet) *spaceUpdateModelOp {
	return &spaceUpdateModelOp{
		subnets: subnets,
		spaceID: spaceID,
	}
}

func (o *spaceUpdateModelOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		for _, subnet := range o.subnets {
			if err := subnet.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	var totalOps []txn.Op
	argToUpdate := network.SubnetInfo{
		SpaceID: o.spaceID,
	}
	for _, subnet := range o.subnets {
		ops, err := subnet.UpdateOps(argToUpdate)
		if err != nil {
			return nil, errors.Trace(err)
		}
		totalOps = append(totalOps, ops...)
	}

	return totalOps, nil
}

// MoveToSpace updates a space by it's given cidr
func (api *API) MoveToSpace(args params.MoveToSpacesParams) (params.MoveToSpaceResults, error) {
	var results params.MoveToSpaceResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return results, err
	}

	results = params.MoveToSpaceResults{
		Results: make([]params.MoveToSpaceResult, len(args.MoveToSpace)),
	}

	for i, updateSpace := range args.MoveToSpace {
		spaceTag, err := names.ParseSpaceTag(updateSpace.SpaceTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		space, err := api.backing.SpaceByName(spaceTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		subnets, err := api.getValidSubnets(updateSpace.CIDRs)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if errString := checkSubnetCIDRAlreadyInSpace(subnets, space); len(errString) > 0 {
			results.Results[i].Error = common.ServerError(errors.Trace(errors.New(strings.Join(errString, "\n"))))
			continue
		}

		operation, err := api.opFactory.NewUpdateSpaceModelOp(space.Id(), subnets)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		if err = api.backing.ApplyOperation(operation); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
	}
	return results, nil
}

func checkSubnetCIDRAlreadyInSpace(subnets []networkingcommon.BackingSubnet, space networkingcommon.BackingSpace) []string {
	var errorStrings []string
	for _, subnet := range subnets {
		if subnet.SpaceID() == space.Id() {
			msg := fmt.Sprintf("supplied CIDR %q is already in space %q", subnet.CIDR(), space.Id())
			errorStrings = append(errorStrings, msg)
		}
	}
	return errorStrings
}
