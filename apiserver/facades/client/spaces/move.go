// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"github.com/juju/juju/state"
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
	CIDR() string
	SpaceName() string
}

// MovedCDIR holds the movement from a CIDR from space `a` to space `b`
type MovedCDIR struct {
	FromSpace string
	CIDR      string
}

// SubnetApplication holds a subnet which belongs to a machine, which can hold n number of applications
type SubnetApplications struct {
	Subnet       networkingcommon.BackingSubnet
	Applications []string
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
func (api *API) MoveToSpace(args params.MoveToSpacesParams) (params.MoveToSpaceResults, error) {
	var results params.MoveToSpaceResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return results, err
	}

	results = params.MoveToSpaceResults{
		Results: make([]params.MoveToSpaceResult, len(args.MoveToSpace)),
	}

	for i, toSpaceParams := range args.MoveToSpace {
		spaceTagTo, err := names.ParseSpaceTag(toSpaceParams.SpaceTagTo)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		subnets, err := api.getValidSubnetsByCIDR(toSpaceParams.CIDRs)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if err := api.checkSubnetAllowedToBeMoved(subnets, toSpaceParams.Force, spaceTagTo); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		operation, err := api.opFactory.NewUpdateSpaceModelOp(spaceTagTo.Id(), subnets)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if err = api.backing.ApplyOperation(operation); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		} else {
			createMovedCidrs(operation.GetMovedCIDRs(), &results.Results[i], spaceTagTo)
			// TODO (nammn): this patch does not include the code about moving subnets and the corresponding endpoints.
			// e.g. mediawiki has slave:db-space with address in CIDR 10.10.10.10/18,
			// moving this CIDR to space fe-space means a reply to the user saying that we may want to move the endpoint binding
			// from db-space to fe-space as well. If we don't do this, this would lead to a case of having a endpoints bound
			// to 2 spaces. Db-space and fe-space.
			// the patch will be added later.
		}

	}
	return results, nil
}

func createMovedCidrs(movedCDIRSpace []MovedCDIR, result *params.MoveToSpaceResult, spaceTo names.SpaceTag) {
	result.Moved = make([]params.MovedSpaceCIDR, len(movedCDIRSpace))
	for i, v := range movedCDIRSpace {
		result.Moved[i].SpaceTagTo = spaceTo.String()
		result.Moved[i].CIDR = v.CIDR
		result.Moved[i].SpaceTagFrom = names.NewSpaceTag(v.FromSpace).String()
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

	subnetsAndApplicationsInUse, err := getSubnetsAndApplicationsInUse(subnets, machines)
	if err != nil {
		return errors.Trace(err)
	}

	return api.checkApplicationConstraints(subnetsAndApplicationsInUse, spaceTag.Id())
}

func (api *API) checkApplicationConstraints(subnetApplications []SubnetApplications, spaceTo string) error {
	constraintsApplicationMap := map[string][]string{}

	constraints, err := api.backing.AllConstraints()
	if err != nil {
		return errors.Trace(err)
	}

	for _, constraint := range constraints {
		tag := state.ParseLocalIDToTags(constraint.ID())
		if tag == nil {
			logger.Debugf("Could not parse tag from constraints ID: %q", constraint.ID())
			continue
		}
		constraintsApplicationMap[tag.Id()] = constraint.Spaces()
	}

	negativeSpaceTo := fmt.Sprintf("^%v", spaceTo)

	for _, subApplication := range subnetApplications {
		for _, application := range subApplication.Applications {
			if spaces, ok := constraintsApplicationMap[application]; ok {
				for _, s := range spaces {
					if s == negativeSpaceTo {
						return errors.Errorf("cannot move CIDR %q"+
							"from space %q to space: %q, as this would"+
							" violate the current application constraint: %q",
							subApplication.Subnet.CIDR(),
							subApplication.Subnet.SpaceName(), negativeSpaceTo)
					}
				}
			}
		}
	}

	return nil
}

// getSubnetsAndApplicationsInUse returns the subnets and corresponding applications which are in use
func getSubnetsAndApplicationsInUse(subnets []networkingcommon.BackingSubnet, machines []Machine) ([]SubnetApplications, error) {
	subnetCIDRs := map[string]networkingcommon.BackingSubnet{}
	for _, subnet := range subnets {
		subnetCIDRs[subnet.CIDR()] = subnet
	}

	var subnetApplications []SubnetApplications

	for _, machine := range machines {
		addresses, err := machine.AllAddresses()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, address := range addresses {
			if subnet, ok := subnetCIDRs[address.SubnetCIDR()]; ok {
				applicationNames, err := machine.ApplicationNames()
				if err != nil {
					return nil, err
				}
				subnetApplications = append(subnetApplications, SubnetApplications{
					Subnet:       subnet,
					Applications: applicationNames,
				})
			}
		}
	}
	return subnetApplications, nil
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
