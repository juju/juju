// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
)

// ApplicationMachines returns the machine IDs of machines which have
// the specified application listed as a principal.
func ApplicationMachines(st *State, application string) ([]string, error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, err
	}
	applicationName := unitAppName(application)
	var machineIds []string
	for _, machine := range machines {
		principalSet := set.NewStrings()
		for _, principal := range machine.Principals() {
			principalSet.Add(unitAppName(principal))
		}
		if principalSet.Contains(applicationName) {
			machineIds = append(machineIds, machine.Id())
		}
	}
	return machineIds, nil
}
