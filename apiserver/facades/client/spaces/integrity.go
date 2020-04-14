// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
)

// unitNetwork represents a group of units and the subnets
// to which they are *all* connected.
type unitNetwork struct {
	unitNames set.Strings
	subnets   network.SubnetInfos
}

// hasSameConnectivity returns true if the input set of subnets
// matches the unitNetwork subnets exactly.
func (n *unitNetwork) hasSameConnectivity(subnets network.SubnetInfos) bool {
	return n.subnets.EqualTo(subnets)
}

// affectedNetworks groups unique unit networks by application name.
// It facilitates checking whether the connectedness of application units
// is able to honour changing space topology based on application
// constraints and endpoint bindings.
type affectedNetworks struct {
	// spaces is a cache of model's complete space topology.
	spaces      network.SpaceInfos
	appNetworks map[string][]unitNetwork
	force       bool
}

func newAffectedNetworks(spaces network.SpaceInfos, force bool) *affectedNetworks {
	return &affectedNetworks{
		spaces:      spaces,
		appNetworks: make(map[string][]unitNetwork),
		force:       force,
	}
}

// includeMachine ensures that the units on the machine and their collection
// of subnet connectedness are included as affectedNetworks to be validated.
func (n *affectedNetworks) includeMachine(machine Machine, subnets network.SubnetInfos) error {
	units, err := machine.Units()
	if err != nil {
		return errors.Trace(err)
	}

	for _, unit := range units {
		appName := unit.ApplicationName()
		unitNets, ok := n.appNetworks[appName]
		if !ok {
			n.appNetworks[appName] = []unitNetwork{}
		}

		var present bool

		for _, unitNet := range unitNets {
			if unitNet.hasSameConnectivity(subnets) {
				unitNet.unitNames.Add(unit.Name())
				present = true
			}
		}

		if !present {
			n.appNetworks[appName] = append(unitNets, unitNetwork{
				unitNames: set.NewStrings(unit.Name()),
				subnets:   subnets,
			})
		}
	}

	return nil
}

func (n *affectedNetworks) ensureSpaceConstraintIntegrity(cons map[string]set.Strings, spaceName string) error {
	for appName, _ := range n.appNetworks {
		spaces, ok := cons[appName]
		if !ok {
			// If the application has no space constraints, we are done.
			continue
		}

		// If the application has a negative space constraint for the
		// destination, the proposed subnet relocation violates it.
		if spaces.Contains("^" + spaceName) {
			msg := fmt.Sprintf("moving subnet(s) to space %q violates space constraints "+
				"for application %q: %s", spaceName, appName, strings.Join(spaces.SortedValues(), ", "))

			if !n.force {
				return errors.New(msg)
			}
			logger.Warningf(msg)
		}
	}

	return nil
}
