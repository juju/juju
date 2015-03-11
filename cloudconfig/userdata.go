// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
)

var (
	logger = loggo.GetLogger("juju.userdata")
)

func configureCloudinit(icfg *instancecfg.InstanceConfig, cloudcfg *cloudinit.Config) (UserdataConfig, error) {
	// When bootstrapping, we only want to apt-get update/upgrade
	// and setup the SSH keys. The rest we leave to cloudinit/sshinit.
	udata, err := NewUserdataConfig(icfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	if icfg.Bootstrap {
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
func ComposeUserData(icfg *instancecfg.InstanceConfig, cloudcfg *cloudinit.Config) ([]byte, error) {
	if cloudcfg == nil {
		cloudcfg = cloudinit.New()
	}
	udata, err := configureCloudinit(icfg, cloudcfg)
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
