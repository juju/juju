// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs/manual/common"
	"github.com/juju/juju/environs/manual/linux"

	"github.com/juju/errors"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
)

// NewScriptProvisioner returns a valid ScriptProvisioner that will be used to produce the script.
func NewScriptProvisioner(icfg *instancecfg.InstanceConfig) (common.ScriptProvisioner, error) {
	seriesos, err := series.GetOSFromSeries(icfg.Series)
	if err != nil {
		return nil, err
	}
	switch seriesos {
	case os.Ubuntu, os.CentOS:
		return linux.NewScript(icfg), nil
	default:
		return nil, errors.NotFoundf(
			"Can't return a valid object for preparing the provisioning script based on this series %q",
			icfg.Series,
		)
	}
}
