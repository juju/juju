// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/utils"

	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/environs/cloudinit"
)

func configureCloudinit(mcfg *cloudinit.InstanceConfig, cloudcfg *coreCloudinit.Config) (cloudinit.UserdataConfig, error) {
	// When bootstrapping, we only want to apt-get update/upgrade
	// and setup the SSH keys. The rest we leave to cloudinit/sshinit.
	udata, err := cloudinit.NewUserdataConfig(mcfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	if mcfg.Bootstrap {
		err = udata.ConfigureBasic()
		if err != nil {
			return nil, err
		}
		return udata, nil
	}
	err = udata.Configure()
	if err != nil {
		return nil, err
	}
	return udata, nil
}

// ComposeUserData fills out the provided cloudinit configuration structure
// so it is suitable for initialising a machine with the given configuration,
// and then renders it and returns it as a binary (gzipped) blob of user data.
//
// If the provided cloudcfg is nil, a new one will be created internally.
func ComposeUserData(mcfg *cloudinit.InstanceConfig, cloudcfg *coreCloudinit.Config) ([]byte, error) {
	if cloudcfg == nil {
		cfg, err := coreCloudinit.New(mcfg.Series)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudcfg = cfg
	}
	udata, err := configureCloudinit(mcfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	data, err := udata.Render()
	logger.Tracef("Generated cloud init:\n%s", string(data))
	if err != nil {
		return nil, err
	}
	return utils.Gzip(data), nil
}
