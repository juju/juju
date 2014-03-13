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

// WriteUserData generates the cloud init for the specified machine config,
// and writes the serialized form out to a cloud-init file in the directory
// specified.
func WriteUserData(machineConfig *cloudinit.MachineConfig, directory string) (string, error) {
	userData, err := cloudInitUserData(machineConfig)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	return WriteCloudInitFile(directory, userData)
}

// WriteCloudInitFile writes the data out to a cloud-init file in the
// directory specified, and returns the filename.
func WriteCloudInitFile(directory string, userData []byte) (string, error) {
	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

func cloudInitUserData(machineConfig *cloudinit.MachineConfig) ([]byte, error) {
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
