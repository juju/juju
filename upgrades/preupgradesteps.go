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
type PreUpgradeStepsFunc func(_ *state.StatePool, _ agent.Config, isController, isMaster bool) error

// PreUpgradeSteps runs various checks and prepares for performing an upgrade.
// If any check fails, an error is returned which aborts the upgrade.
func PreUpgradeSteps(pool *state.StatePool, agentConf agent.Config, isController, isMaster bool) error {
	if err := CheckFreeDiskSpace(agentConf.DataDir(), MinDiskSpaceMib); err != nil {
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

// MinDiskSpaceMib is the standard amount of disk space free (in MiB)
// we'll require before downloading a binary or starting an upgrade.
var MinDiskSpaceMib = uint64(250)

// CheckFreeDiskSpace returns a helpful error if there isn't at
// least thresholdMib MiB of free space available on the volume
// containing dir.
func CheckFreeDiskSpace(dir string, thresholdMib uint64) error {
	usage := du.NewDiskUsage(dir)
	available := usage.Available()
	if available < thresholdMib*humanize.MiByte {
		return errors.Errorf("not enough free disk space on %q for upgrade: %s available, require %dMiB",
			dir, humanize.IBytes(available), thresholdMib)
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
