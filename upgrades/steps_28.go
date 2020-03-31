// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/service"
	"github.com/juju/juju/worker/common/reboot"
)

// stateStepsFor28 returns upgrade steps for Juju 2.8.0.
func stateStepsFor28() []Step {
	return []Step{
		&upgradeStep{
			description: "drop old presence database",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DropPresenceDatabase()
			},
		},
		&upgradeStep{
			description: "increment tasks sequence by 1",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().IncrementTasksSequence()
			},
		},
		&upgradeStep{
			description: "add machine ID to subordinate units",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddMachineIDToSubordinates()
			},
		},
	}
}

// stepsFor28 returns upgrade steps for Juju 2.8.0.
func stepsFor28() []Step {
	return []Step{
		// This step pre-populates the reboot-handled flag for all
		// running units so they do not accidentally trigger a start
		// hook once they get restarted after the upgrade is complete.
		&upgradeStep{
			description: "ensure currently running units do not fire start hooks thinking a reboot has occurred",
			targets:     []Target{HostMachine},
			run:         prepopulateRebootHandledFlagsForDeployedUnits,
		},
	}
}

func prepopulateRebootHandledFlagsForDeployedUnits(ctx Context) error {
	// Lookup the names of all unit agents installed on this machine.
	agentConf := ctx.AgentConfig()
	_, unitNames, _, err := service.FindAgents(agentConf.DataDir())
	if err != nil {
		return errors.Annotate(err, "looking up unit agents")
	}

	// Pre-populate reboot-handled flag files.
	monitor := reboot.NewMonitor(agentConf.TransientDataDir())
	for _, unitName := range unitNames {
		// This should never fail as it is already validated by the
		// FindAgents call. However, since this is an upgrade step
		// it's fine to be a bit extra paranoid.
		tag, err := names.ParseTag(unitName)
		if err != nil {
			return errors.Annotatef(err, "unable to parse unit agent tag %q", unitName)
		}

		// Querying the reboot monitor will set up the flag for this
		// unit if it doesn't already exist.
		if _, err = monitor.Query(tag); err != nil {
			return errors.Annotatef(err, "querying reboot monitor for %q", unitName)
		}
	}
	return nil
}
