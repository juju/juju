//Copyright 2020 Canonical Ltd.
//Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
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

// UpdateSpace updates a space by it's given cidr
func (api *API) UpdateSpace(args params.UpdateSpacesParams) (params.ErrorResults, error) {
	var errorResults params.ErrorResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return errorResults, err
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.UpdateSpaces)),
	}

	for i, updateSpace := range args.UpdateSpaces {
		spaceTag, err := names.ParseSpaceTag(updateSpace.SpaceTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		// space exist
		// TODO: may check space.subnets() == cidrs and handle that
		_, err = api.backing.SpaceByName(spaceTag.String())
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		// spaceID exist and cidr are fine
		subnets, err := api.getValidSubnets(updateSpace.CIDRs)
		if err != nil {
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
		}
	}
	return results, nil
}
