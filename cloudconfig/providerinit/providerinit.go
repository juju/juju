// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// This package offers userdata in a gzipped format to be used by different
// cloud providers
package providerinit

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cloudconfig.providerinit")

func configureCloudinit(icfg *instancecfg.InstanceConfig, cloudcfg cloudinit.CloudConfig) (cloudconfig.UserdataConfig, error) {
	// When bootstrapping, we only want to apt-get update/upgrade
	// and setup the SSH keys. The rest we leave to cloudinit/sshinit.
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
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
func ComposeUserData(icfg *instancecfg.InstanceConfig, cloudcfg cloudinit.CloudConfig) ([]byte, error) {
	if cloudcfg == nil {
		var err error
		cloudcfg, err = cloudinit.New(icfg.Series)
		if err != nil {
			return nil, err
		}
	}
	_, err := configureCloudinit(icfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	operatingSystem, err := version.GetOSFromSeries(icfg.Series)
	if err != nil {
		return nil, err
	}
	switch operatingSystem {
	case version.Ubuntu, version.CentOS:
		return gzippedUserdata(cloudcfg)
	case version.Windows:
		return encodedUserdata(cloudcfg)
	default:
		return nil, errors.New(fmt.Sprintf("Cannot compose userdata for os %s", operatingSystem))
	}
}

// gzippedUserdata returns the rendered userdata in a gzipped format
func gzippedUserdata(cloudcfg cloudinit.CloudConfig) ([]byte, error) {
	data, err := cloudcfg.RenderYAML()
	logger.Tracef("Generated cloud init:\n%s", string(data))
	if err != nil {
		return nil, err
	}
	return utils.Gzip(data), nil
}

// encodedUserdata for now is used on windows and it retuns a powershell script
// which has the userdata embedded as base64(gzip(userdata))
// We need this because most cloud provider do not accept gzipped userdata on
// windows and they have size limitations
func encodedUserdata(cloudcfg cloudinit.CloudConfig) ([]byte, error) {
	zippedData, err := gzippedUserdata(cloudcfg)
	if err != nil {
		return nil, err
	}

	base64Data := base64.StdEncoding.EncodeToString(zippedData)
	return []byte(fmt.Sprintf(cloudconfig.UserdataScript, base64Data)), nil
}
