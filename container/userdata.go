// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/loggo"

	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/environs/cloudinit"
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
	udata, err := cloudinit.NewUserdataConfig(machineConfig, cloudConfig)
	if err != nil {
		return nil, err
	}
	err = udata.Configure()
	if err != nil {
		return nil, err
	}
	// Run ifconfig to get the addresses of the internal container at least
	// logged in the host.
	cloudConfig.AddRunCmd("ifconfig")

	renderer, err := coreCloudinit.NewRenderer(machineConfig.Series)
	if err != nil {
		return nil, err
	}

	data, err := renderer.Render(cloudConfig)
	if err != nil {
		return nil, err
	}
	return data, nil
}
