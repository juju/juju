// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MovingSubnetBacking describes the state backing required to move a subnet
type MovingSubnetBacking interface {
	UpdateSubnetSpaceOps(string, string) []txn.Op
}

// MovingSubnet describes a subnet that is to be relocated to a new space.
type MovingSubnet interface {
	ID() string
	CIDR() string
	SpaceName() string
	SpaceID() string
	FanLocalUnderlay() string

	Refresh() error
}

// MovedSubnet identifies a subnet and the space it was moved from.
type MovedSubnet struct {
	ID        string
	FromSpace string
	CIDR      string
}

// MoveSubnetsOp describes a model operation for moving subnets to a new space.
type MoveSubnetsOp interface {
	state.ModelOperation

	// GetMovedSubnets returns the information for subnets that were
	// successfully moved as a result of applying this operation.
	GetMovedSubnets() []MovedSubnet
}

// moveSubnetsOp implements MoveSubnetsOp.
// It is a model operation that updates the subnets collection to indicate
// subnets moving from one space to another.
type moveSubnetsOp struct {
	backing MovingSubnetBacking
	spaceID string
	subnets []MovingSubnet
	results []MovedSubnet
}

// NewMoveSubnetsOp returns an operation reference that can be
// used to move the input subnets into the input space.
func NewMoveSubnetsOp(
	backing MovingSubnetBacking, spaceID string, subnets []MovingSubnet,
) *moveSubnetsOp {
	return &moveSubnetsOp{
		backing: backing,
		spaceID: spaceID,
		subnets: subnets,
	}
}

// Build (ModelOperation) returns a collection of transaction operations
// that will modify state to indicate the movement of subnets.
func (o *moveSubnetsOp) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		for _, subnet := range o.subnets {
			if err := subnet.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	var ops []txn.Op
	for _, subnet := range o.subnets {
		ops = append(ops, o.backing.UpdateSubnetSpaceOps(subnet.ID(), o.spaceID)...)
	}
	return ops, nil
}

// Done (ModelOperation) is called upon execution of the operations returned by
// Build. It records the successfully moved subnets for later retrieval.
func (o *moveSubnetsOp) Done(err error) error {
	if err == nil {
		o.results = make([]MovedSubnet, len(o.subnets))
		for i, subnet := range o.subnets {
			mc := MovedSubnet{
				ID:        subnet.ID(),
				FromSpace: subnet.SpaceName(),
				CIDR:      subnet.CIDR(),
			}
			o.results[i] = mc
		}
	}
	return err
}

// GetMovedSubnets (MoveSubnetsOp) returns the results of successfully
// executed movement of subnets to a new space.
func (o *moveSubnetsOp) GetMovedSubnets() []MovedSubnet {
	return o.results
}

// MoveSubnets ensures that the input subnets are in the input space.
func (api *API) MoveSubnets(ctx context.Context, args params.MoveSubnetsParams) (params.MoveSubnetsResults, error) {
	var result params.MoveSubnetsResults

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	results := make([]params.MoveSubnetsResult, len(args.Args))
	for i, toSpaceParams := range args.Args {
		// Note that although spaces have an ID, a space tag represents
		// a space *name*, which remains a unique identifier.
		// We need to retrieve the space in order to use its ID.
		spaceTag, err := names.ParseSpaceTag(toSpaceParams.SpaceTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		spaceName := spaceTag.Id()

		subnets, err := api.getMovingSubnets(toSpaceParams.SubnetTags)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		if err := api.ensureSubnetsCanBeMoved(subnets, spaceName, toSpaceParams.Force); err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		operation, err := api.opFactory.NewMoveSubnetsOp(spaceName, subnets)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		if err = api.backing.ApplyOperation(operation); err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		results[i].NewSpaceTag = spaceTag.String()
		results[i].MovedSubnets = paramsFromMovedSubnet(operation.GetMovedSubnets())
	}

	result.Results = results
	return result, nil
}

// getMovingSubnets acquires all the subnets that we have
// been requested to relocate, identified by their tags.
func (api *API) getMovingSubnets(tags []string) ([]MovingSubnet, error) {
	subnets := make([]MovingSubnet, len(tags))
	for i, tag := range tags {
		subTag, err := names.ParseSubnetTag(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnet, err := api.backing.MovingSubnet(subTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnets[i] = subnet
	}
	return subnets, nil
}

// ensureSubnetsCanBeMoved gathers the relevant networking info required to
// determine the validity of constraints and endpoint bindings resulting from
// a relocation of subnets.
// An error is returned if validity is violated and force is passed as false.
func (api *API) ensureSubnetsCanBeMoved(subnets []MovingSubnet, spaceName string, force bool) error {
	for _, subnet := range subnets {
		if subnet.FanLocalUnderlay() != "" {
			return errors.Errorf("subnet %q is a fan overlay of %q and cannot be moved; move the underlay instead",
				subnet.CIDR(), subnet.FanLocalUnderlay())
		}
	}

	affected, err := api.getAffectedNetworks(subnets, spaceName, force)
	if err != nil {
		return errors.Annotate(err, "determining affected networks")
	}

	if err := api.ensureSpaceConstraintIntegrity(affected); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(api.ensureEndpointBindingsIntegrity(affected))
}

// getAffectedNetworks interrogates machines connected to moving subnets.
// From these it generates lists of common unit/subnet-topologies,
// grouped by application.
func (api *API) getAffectedNetworks(subnets []MovingSubnet, spaceName string, force bool) (*affectedNetworks, error) {
	movingSubnetIDs := network.MakeIDSet()
	for _, subnet := range subnets {
		movingSubnetIDs.Add(network.Id(subnet.ID()))
	}

	allSpaces, err := api.backing.AllSpaceInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}

	affected, err := newAffectedNetworks(movingSubnetIDs, spaceName, allSpaces, force, api.logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machines, err := api.backing.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := affected.processMachines(machines); err != nil {
		return nil, errors.Annotate(err, "processing machine networks")
	}

	return affected, nil
}

// ensureSpaceConstraintIntegrity identifies all applications connected to
// subnets that we have been asked to move.
// It then compares any space constraints that these applications have against
// the requested destination space, to check if they will have continuity of
// those constraints after subnet relocation.
// If force is true we only log a warning for violations, otherwise an error
// is returned.
func (api *API) ensureSpaceConstraintIntegrity(affected *affectedNetworks) error {
	constraints, err := api.backing.AllConstraints()
	if err != nil {
		return errors.Trace(err)
	}

	// Create a lookup of constrained space names by application.
	spaceConsByApp := make(map[string]set.Strings)
	for _, cons := range constraints {
		// Get the tag for the entity to which this constraint applies.
		tag := state.TagFromDocID(cons.ID())
		if tag == nil {
			return errors.Errorf("unable to determine an entity to which constraint %q applies", cons.ID())
		}

		// We only care if this is an application constraint,
		// and it includes spaces.
		val := cons.Value()
		if tag.Kind() == names.ApplicationTagKind && val.HasSpaces() {
			spaceCons := val.Spaces
			spaceConsByApp[tag.Id()] = set.NewStrings(*spaceCons...)
		}
	}

	return errors.Trace(affected.ensureConstraintIntegrity(spaceConsByApp))
}

func (api *API) ensureEndpointBindingsIntegrity(affected *affectedNetworks) error {
	allBindings, err := api.backing.AllEndpointBindings()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(affected.ensureBindingsIntegrity(allBindings))
}

func paramsFromMovedSubnet(movedSubnets []MovedSubnet) []params.MovedSubnet {
	results := make([]params.MovedSubnet, len(movedSubnets))
	for i, v := range movedSubnets {
		results[i] = params.MovedSubnet{
			SubnetTag:   names.NewSubnetTag(v.ID).String(),
			OldSpaceTag: names.NewSpaceTag(v.FromSpace).String(),
			CIDR:        v.CIDR,
		}
	}
	return results
}
