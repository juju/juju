// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/utils/series"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
)

// stepsFor245 returns upgrade steps for Juju 2.4.5
func stepsFor245() []Step {
	return []Step{
		&upgradeStep{
			description: "update exec.start.sh log path if incorrect",
			targets:     []Target{AllMachines},
			run:         correctServiceFileLogPath,
		},
	}
}

// install the service files in Standard location - '/lib/systemd/system path.
func correctServiceFileLogPath(context Context) error {
	hostSeries, err := series.HostSeries()
	if err != nil {
		logger.Errorf("getting host series: %e", err)
	}
	initName, err := service.VersionInitSystem(hostSeries)
	if err != nil {
		logger.Errorf("unsuccessful checking init script for correct log path: %e", err)
		return err
	}
	if initName != service.InitSystemSystemd {
		return nil
	}
	// rewrite files to correct errors in previous upgrade step
	sysdManager := service.NewSystemdServiceManager(systemd.IsRunning)
	err = sysdManager.WriteServiceFile()
	if err != nil {
		logger.Errorf("rewriting service file: %e", err)
		return err
	}
	return nil
}
