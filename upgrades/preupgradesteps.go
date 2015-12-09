// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/utils/du"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

// PreUpgradeSteps runs various checks and prepares for performing an upgrade.
// If any check fails, an error is returned which aborts the upgrade.
func PreUpgradeSteps(st *state.State, agentConf agent.Config, isMaster bool) error {
	if err := checkDiskSpace(agentConf.DataDir()); err != nil {
		return err
	}
	return nil
}

// We'll be conservative and require at least 2GiB of disk space for an upgrade.
var MinDiskSpaceGib = 2

func checkDiskSpace(dir string) error {
	usage := du.NewDiskUsage(dir)
	free := usage.Free()
	if free < uint64(MinDiskSpaceGib*humanize.GiByte) {
		return errors.Errorf("not enough free disk space for upgrade: %s available, require %dGiB",
			humanize.IBytes(free), MinDiskSpaceGib)
	}
	return nil
}
