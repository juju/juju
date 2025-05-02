// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

// unitNetwork represents a group of units and the subnets
// to which they are *all* connected.
type unitNetwork struct {
	unitNames set.Strings
	// subnets are those that all the units are connected to.
	// Not that these are populated from the *current* network topology.
	subnets network.SubnetInfos
}

// hasSameConnectivity returns true if the input set of subnets
// matches the unitNetwork subnets exactly, based on ID.
func (n *unitNetwork) hasSameConnectivity(subnets network.SubnetInfos) bool {
	return n.subnets.EqualTo(subnets)
}

// remainsConnectedTo returns true if the unitNetwork has a subnet in common
// with the input future representation of the target space.
// Note that we compare subnet IDs and not space IDs, because the subnets are
// populated from the existing topology, whereas the input space comes from
// the hypothetical new topology where one or more subnets have moved there.
func (n *unitNetwork) remainsConnectedTo(space network.SpaceInfo) bool {
	spaceSubs := space.Subnets
	for _, sub := range n.subnets {
		if spaceSubs.ContainsID(sub.ID) {
			return true
		}
	}
	return false
}

// affectedNetworks groups unique unit networks by application name.
// It facilitates checking whether the connectedness of application units
// is able to honour changing space topology based on application
// constraints and endpoint bindings.
type affectedNetworks struct {
	// subnets identifies the subnets that are being moved.
	subnets network.IDSet
	// newSpace is the name of the space that the subnets are being moved to.
	newSpace string
	// spaces is a the target space topology.
	spaces network.SpaceInfos
	// changingNetworks is all unit subnet connectivity grouped by application
	// for any that may be affected by moving the subnets above.
	changingNetworks map[string][]unitNetwork
	// unchangedNetworks is all unit subnet connectivity grouped by application
	// for those that are unaffected by moving subnets.
	// These are included in order to determine whether application endpoint
	// bindings can be massaged to satisfy the mutating space topology.
	unchangedNetworks map[string][]unitNetwork
	// force originates as a CLI option.
	// When true, violations of constraints/bindings integrity are logged as
	// warnings instead of being returned as errors.
	force              bool
	logger             corelogger.Logger
	applicationService ApplicationService
}

// newAffectedNetworks returns a new affectedNetworks reference for
// verification of the movement of the input subnets to the input space.
// The input space topology is manipulated to represent the topology that
// would result from the move.
func newAffectedNetworks(
	applicationService ApplicationService, movingSubnets network.IDSet, spaceName string, currentTopology network.SpaceInfos, force bool, logger corelogger.Logger,
) (*affectedNetworks, error) {
	// Get the topology as would result from moving all of these subnets.
	newTopology, err := currentTopology.MoveSubnets(movingSubnets, spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &affectedNetworks{
		subnets:            movingSubnets,
		newSpace:           spaceName,
		spaces:             newTopology,
		changingNetworks:   make(map[string][]unitNetwork),
		unchangedNetworks:  make(map[string][]unitNetwork),
		force:              force,
		logger:             logger,
		applicationService: applicationService,
	}, nil
}

// processMachines iterates over the input machines,
// looking at the subnets they are connected to.
// Any machines connected to a moving subnet have their unit networks
// included for for later verification.
func (n *affectedNetworks) processMachines(ctx context.Context, machines []Machine) error {
	for _, machine := range machines {
		addresses, err := machine.AllAddresses()
		if err != nil {
			return errors.Trace(err)
		}

		var includesMover bool
		var machineSubnets network.SubnetInfos
		for _, address := range addresses {
			// These are not going to have subnets, so just ignore them.
			if address.ConfigMethod() == network.ConfigLoopback {
				continue
			}

			sub, err := n.addressSubnet(address)
			if err != nil {
				return errors.Trace(err)
			}
			machineSubnets = append(machineSubnets, sub)

			if n.subnets.Contains(sub.ID) {
				includesMover = true
			}
		}

		if err = n.includeMachine(ctx, machine, machineSubnets, includesMover); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (n *affectedNetworks) addressSubnet(addr Address) (network.SubnetInfo, error) {
	allSubs, err := n.spaces.AllSubnetInfos()
	if err != nil {
		return network.SubnetInfo{}, errors.Trace(err)
	}

	subs, err := allSubs.GetByCIDR(addr.SubnetCIDR())
	if err != nil {
		return network.SubnetInfo{}, errors.Trace(err)
	}

	// TODO (manadart 2020-05-07): This is done on the basis of CIDR still
	// uniquely identifying a subnet.
	// It will have to change for multi-network enablement.
	if len(subs) > 0 {
		return subs[0], nil
	}

	// If the address CIDR was not located in our network topology,
	// it *may* be due to the fact that fan addresses are indicated as being
	// part of their *overlay* (such as 252.0.0.0/8), rather than the
	// zone-specific segments of the overlay (such as 252.32.0.0/12)
	// as seen in AWS.
	// Try to locate a subnet based on the address itself.
	subs, err = allSubs.GetByAddress(addr.Value())
	if err != nil {
		return network.SubnetInfo{}, errors.Trace(err)
	}

	if len(subs) > 0 {
		return subs[0], nil
	}
	return network.SubnetInfo{}, errors.NotFoundf("subnet for machine address %q", addr.Value())
}

// includeMachine ensures that the units on the machine and their collection
// of subnet connectedness are included as networks to be validated.
// The collection they are placed into depends on whether they are connected to
// a moving subnet, indicated by the netChange argument.
func (n *affectedNetworks) includeMachine(ctx context.Context, machine Machine, subnets network.SubnetInfos, netChange bool) error {
	machineName := coremachine.Name(machine.Id())
	unitNames, err := n.applicationService.GetUnitNamesOnMachine(ctx, machineName)
	if errors.Is(err, applicationerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", machineName)
	} else if err != nil {
		return errors.Trace(err)
	}

	collection := n.unchangedNetworks
	if netChange {
		collection = n.changingNetworks
	}

	for _, unitName := range unitNames {
		appName := unitName.Application()
		unitNets, ok := collection[appName]
		if !ok {
			collection[appName] = []unitNetwork{}
		}

		var present bool

		for _, unitNet := range unitNets {
			if unitNet.hasSameConnectivity(subnets) {
				unitNet.unitNames.Add(unitName.String())
				present = true
			}
		}

		if !present {
			collection[appName] = append(unitNets, unitNetwork{
				unitNames: set.NewStrings(unitName.String()),
				subnets:   subnets,
			})
		}
	}

	return nil
}

// ensureConstraintIntegrity checks that moving subnets to the new space does
// not violate any application space constraints.
func (n *affectedNetworks) ensureConstraintIntegrity(ctx context.Context, cons map[string]set.Strings) error {
	for appName, spaces := range cons {
		if _, ok := n.changingNetworks[appName]; !ok {
			// The constraint is for an application not affected by the move.
			continue
		}

		if err := n.ensureNegativeConstraintIntegrity(ctx, appName, spaces); err != nil {
			return errors.Trace(err)
		}

		if err := n.ensurePositiveConstraintIntegrity(ctx, appName, spaces); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// ensureNegativeConstraintIntegrity checks that the input application does not
// have a negative space constraint for the proposed destination space.
func (n *affectedNetworks) ensureNegativeConstraintIntegrity(ctx context.Context, appName string, spaceConstraints set.Strings) error {
	if spaceConstraints.Contains("^" + n.newSpace) {
		msg := fmt.Sprintf("moving subnet(s) to space %q violates space constraints "+
			"for application %q: %s", n.newSpace, appName, strings.Join(spaceConstraints.SortedValues(), ", "))

		if !n.force {
			return errors.New(msg)
		}
		n.logger.Warningf(ctx, msg)
	}

	return nil
}

// ensurePositiveConstraintIntegrity checks that for each positive space
// constraint, comparing the input application's unit subnet connectivity to
// the target topology determines the constraint to be satisfied.
func (n *affectedNetworks) ensurePositiveConstraintIntegrity(ctx context.Context, appName string, spaceConstraints set.Strings) error {
	unitNets := n.changingNetworks[appName]

	for _, spaceName := range spaceConstraints.Values() {
		if strings.HasPrefix(spaceName, "^") {
			continue
		}

		conSpace := n.spaces.GetByName(spaceName)
		if conSpace == nil {
			return errors.NotFoundf("space with name %q", spaceName)
		}

		for _, unitNet := range unitNets {
			if unitNet.remainsConnectedTo(*conSpace) {
				continue
			}

			msg := fmt.Sprintf(
				"moving subnet(s) to space %q violates space constraints for application %q: %s\n\t"+
					"units not connected to the space: %s",
				n.newSpace,
				appName,
				strings.Join(spaceConstraints.SortedValues(), ", "),
				strings.Join(unitNet.unitNames.SortedValues(), ", "),
			)

			if !n.force {
				return errors.New(msg)
			}
			n.logger.Warningf(ctx, msg)
		}
	}

	return nil
}

// ensureBindingsIntegrity checks that moving subnets to the new space does
// not result in inconsistent application endpoint bindings.
// Consistency is considered maintained if:
//  1. Bound spaces remain unchanged by subnet relocation.
//  2. We successfully change affected bindings to a new space that
//     preserves consistency across all units of an application.
func (n *affectedNetworks) ensureBindingsIntegrity(ctx context.Context, allBindings map[string]Bindings) error {
	for appName, bindings := range allBindings {
		if err := n.ensureApplicationBindingsIntegrity(ctx, appName, bindings); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (n *affectedNetworks) ensureApplicationBindingsIntegrity(ctx context.Context, appName string, appBindings Bindings) error {
	unitNets, ok := n.changingNetworks[appName]
	if !ok {
		return nil
	}

	for endpoint, boundSpaceID := range appBindings.Map() {
		for _, unitNet := range unitNets {
			boundSpace := n.spaces.GetByID(boundSpaceID)
			if boundSpace == nil {
				return errors.NotFoundf("space with ID %q", boundSpaceID)
			}

			// TODO (manadart 2020-05-05): There is some optimisation that
			// could be done here to prevent re-checking endpoints bound to
			// spaces that we have already checked, but at this time it is
			// eschewed for clarity.
			// The comparisons are all done using the in-memory topology,
			// without going back to the DB, so it is not a huge issue.
			if unitNet.remainsConnectedTo(*boundSpace) {
				continue
			}

			// TODO (manadart 2020-05-05): At this point,
			// use n.unchangedNetworks in combination with n.changingNetworks
			// to see if we can maintain integrity by changing the binding to
			// the target space. If we can, just make the change and log it.

			msg := fmt.Sprintf(
				"moving subnet(s) to space %q violates endpoint binding %s:%s for application %q\n\t"+
					"units not connected to the space: %s",
				n.newSpace,
				endpoint,
				boundSpace.Name,
				appName,
				strings.Join(unitNet.unitNames.SortedValues(), ", "),
			)

			if !n.force {
				return errors.New(msg)
			}
			n.logger.Warningf(ctx, msg)
		}
	}

	return nil
}
