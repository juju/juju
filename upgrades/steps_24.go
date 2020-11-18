// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
)

// stateStepsFor24 returns upgrade steps for Juju 2.4.0 that manipulate state directly.
func stateStepsFor24() []Step {
	return []Step{
		&upgradeStep{
			description: "move or drop the old audit log collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveOldAuditLog()
			},
		},
		&upgradeStep{
			description: "move controller info Mongo space to controller config HA space if valid",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveMongoSpaceToHASpaceConfig()
			},
		},
		&upgradeStep{
			description: "create empty application settings for all applications",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CreateMissingApplicationConfig()
			},
		},
		&upgradeStep{
			description: "remove votingmachineids",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveVotingMachineIds()
			},
		},
		&upgradeStep{
			description: "add cloud model counts",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddCloudModelCounts()
			},
		},
		&upgradeStep{
			description: "bootstrap raft cluster",
			targets:     []Target{Controller},
			run:         BootstrapRaft,
		},
	}
}

// stepsFor24 returns upgrade steps for Juju 2.4.
func stepsFor24() []Step {
	return []Step{
		&upgradeStep{
			description: "Install the services file in standard location '/etc/systemd'",
			targets:     []Target{AllMachines},
			run:         writeServiceFiles(true),
		},
	}
}

// writeServiceFiles writes service files into the default systemd search path.
// The supplied boolean indicates whether the old
// /var/lib/init files should be removed.
func writeServiceFiles(cleanupOld bool) func(Context) error {
	return func(ctx Context) error {
		hostSeries, err := series.HostSeries()
		if err != nil {
			return errors.Trace(err)
		}

		initName, err := service.VersionInitSystem(hostSeries)
		if err != nil {
			return errors.Annotate(err, "writing systemd service files")
		}

		if initName == service.InitSystemSystemd {
			if err := WriteServiceFiles(service.NewServiceManagerWithDefaults()); err != nil {
				return errors.Annotate(err, "writing systemd service files")
			}

			if cleanupOld {
				return errors.Trace(os.RemoveAll(filepath.Join(ctx.AgentConfig().DataDir(), "init")))
			}
			return nil
		}

		logger.Infof("skipping upgrade for non systemd series %s", hostSeries)
		return nil
	}
}

// WriteServiceFiles writes service files to the standard
// /etc/systemd/system path. This implementation is moved here to allow
// the upgrade from very early 2.x to 2.9.
func WriteServiceFiles(s service.SystemdServiceManager) error {
	machineAgent, unitAgents, _, err := s.FindAgents(paths.NixDataDir)
	if err != nil {
		return errors.Trace(err)
	}

	for _, name := range append([]string{machineAgent}, unitAgents...) {
		err := s.WriteSystemdAgent(name, paths.NixDataDir, systemd.EtcSystemdMultiUserDir)
		if err != nil {
			return errors.Annotate(err, name)
		}
	}

	return errors.Trace(systemd.SysdReload())
}
