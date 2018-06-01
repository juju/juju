// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/packaging/manager"
	"github.com/juju/utils/du"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

// PreUpgradeStepsFunc is the function type of PreUpgradeSteps. This may be
// used to provide an alternative to PreUpgradeSteps to the upgrade steps
// worker.
type PreUpgradeStepsFunc func(_ *state.State, _ agent.Config, isController, isMaster bool) error

// PreUpgradeSteps runs various checks and prepares for performing an upgrade.
// If any check fails, an error is returned which aborts the upgrade.
func PreUpgradeSteps(st *state.State, agentConf agent.Config, isController, isMaster bool) error {
	if err := checkDiskSpace(agentConf.DataDir()); err != nil {
		return errors.Trace(err)
	}
	if isController {
		// Update distro info in case the new Juju controller version
		// is aware of new supported series. We'll keep going if this
		// fails, and the user can manually update it if they need to.
		logger.Infof("updating distro-info")
		err := updateDistroInfo()
		return errors.Annotate(err, "failed to update distro-info")
	}
	return nil
}

// MinDiskSpaceMib defines the minimum disk space required
// We'll be conservative and require at least 250MiB of disk space for an upgrade.
var MinDiskSpaceMib = uint64(250)

func checkDiskSpace(dir string) error {
	usage := du.NewDiskUsage(dir)
	free := usage.Free()
	if free < MinDiskSpaceMib*humanize.MiByte {
		return errors.Errorf("not enough free disk space for upgrade: %s available, require %dMiB",
			humanize.IBytes(free), MinDiskSpaceMib)
	}
	return nil
}

func updateDistroInfo() error {
	pm := manager.NewAptPackageManager()
	if err := pm.Update(); err != nil {
		return errors.Annotate(err, "updating package list")
	}
	if err := pm.Install("distro-info"); err != nil {
		return errors.Annotate(err, "updating distro-info package")
	}
	return series.UpdateSeriesVersions()
}
