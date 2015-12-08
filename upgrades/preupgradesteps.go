// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/utils/du"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// PreUpgradeSteps runs various checks and prepares for performing an upgrade.
// If any check fails, an error is returned which aborts the upgrade.
func PreUpgradeSteps(st *state.State, agentConf agent.Config, isMaster bool) error {
	if err := checkDiskSpace(agentConf.DataDir()); err != nil {
		return err
	}
	if isMaster {
		if err := checkProviderAPI(st); err != nil {
			return err
		}
	}
	return nil
}

// We'll be conservative and require at least 2GiB of disk space for an upgrade.
var minDiskSpaceGib = 2

func checkDiskSpace(dir string) error {
	usage := du.NewDiskUsage(dir)
	free := usage.Free()
	if free < uint64(minDiskSpaceGib*humanize.GiByte) {
		return errors.Errorf("not enough free disk space for upgrade %dGiB available, require %dGiB",
			free/humanize.GiByte, minDiskSpaceGib)
	}
	return nil
}

func checkProviderAPI(st *state.State) error {
	// We will make a simple API call to the provider
	// to ensure the underlying substrate is ok.
	env, err := getEnvironment(st)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = env.AllInstances()
	if err != nil {
		return errors.Annotate(err, "cannot make API call to provider")
	}
	return nil
}

var getEnvironment = func(st *state.State) (environs.Environ, error) {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}
