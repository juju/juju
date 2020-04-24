// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/api/upgradesteps"
	"github.com/juju/names/v4"
)

// stepsFor263 returns upgrade steps for Juju 2.6.3.
func stepsFor263() []Step {
	return []Step{
		&upgradeStep{
			description: "reset kvm machine modification status to idle",
			targets:     []Target{AllMachines},
			run:         resetKVMMachineModificationStatusIdle,
		},
	}
}

func resetKVMMachineModificationStatusIdle(context Context) error {
	tag := context.AgentConfig().Tag()
	if !names.IsContainerMachine(tag.Id()) {
		// Skip if not a container, work to be done only on KVM
		// machines.
		return nil
	}
	client := upgradesteps.NewClient(context.APIState())
	return client.ResetKVMMachineModificationStatusIdle(tag)
}

// stateStepsFor263 returns upgrade steps for Juju 2.6.3.
func stateStepsFor263() []Step {
	return []Step{
		&upgradeStep{
			description: "update model name index of k8s models",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().UpdateK8sModelNameIndex()
			},
		},
	}
}
