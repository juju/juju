// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit

import (
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
)

var logger = loggo.GetLogger("juju.cloudconfig.containerinit")

func CloudInitUserData(
	cloudConfig cloudinit.CloudConfig,
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	var interfaces corenetwork.InterfaceInfos
	if networkConfig != nil {
		interfaces = networkConfig.Interfaces
	}

	if err := cloudConfig.AddNetworkConfig(interfaces); err != nil {
		return nil, errors.Trace(err)
	}

	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = udata.Configure(); err != nil {
		return nil, errors.Trace(err)
	}

	// Run ifconfig/ip addr to get the addresses of the
	// internal container at least logged in the host.
	cloudConfig.AddRunCmd("ifconfig || ip addr")

	if instanceConfig.MachineContainerHostname != "" {
		logger.Debugf("Cloud-init configured to set hostname")
		cloudConfig.SetAttr("hostname", instanceConfig.MachineContainerHostname)
	}

	data, err := cloudConfig.RenderYAML()
	return data, errors.Trace(err)
}
