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
	appNetworks map[string][]unitNetwork
}

func newAffectedNetworks() *affectedNetworks {
	return &affectedNetworks{
		appNetworks: make(map[string][]unitNetwork),
	}
}

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

func (n *affectedNetworks) ensureSpaceConstraintIntegrity(
	cons map[string]set.Strings, spaceName string, force bool,
) error {
	for appName, _ := range n.appNetworks {
		if spaces, ok := cons[appName]; ok {
			// If the application has a negative space constraint for the
			// destination, the proposed subnet relocation violates it.
			if spaces.Contains("^" + spaceName) {
				msg := fmt.Sprintf("moving subnet(s) to space %q violates space constraints "+
					"for application %q: %s", spaceName, appName, strings.Join(spaces.SortedValues(), ", "))

				if !force {
					return errors.New(msg)
				}
				logger.Warningf(msg)
			}

			// For each unique set of connected subnets for units in the application,
			// Check that that positive space constraints remain satisfied by relocating
			// subnets to the input space.
		}
	}

	return nil
}
