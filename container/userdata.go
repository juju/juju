// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs/cloudinit"
)

var (
	logger = loggo.GetLogger("juju.container")
)

func WriteUserData(machineConfig *cloudinit.MachineConfig, directory string) (string, error) {
	userData, err := cloudInitUserData(machineConfig)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

func cloudInitUserData(machineConfig *cloudinit.MachineConfig) ([]byte, error) {
	// consider not having this line hardcoded...
	machineConfig.DataDir = "/var/lib/juju"
	cloudConfig := coreCloudinit.New()
	err := cloudinit.Configure(machineConfig, cloudConfig)
	if err != nil {
		return nil, err
	}

	// Run ifconfig to get the addresses of the internal container at least
	// logged in the host.
	cloudConfig.AddRunCmd("ifconfig")

	data, err := cloudConfig.Render()
	if err != nil {
		return nil, err
	}
	return data, nil
}
