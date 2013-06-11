// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/golxc"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate = "ubuntu-cloud"
	containerDir    = "/var/lib/juju/containers"
)

type lxcContainer struct {
	*golxc.Container
	machine *state.Machine
}

func NewContainer(st *state.State, machineId string) (container.Container, error) {
	// TODO(thumper): RSN™ use the API to get the machine.
	machine, err := st.Machine(machineId)
	if err != nil {
		logger.Errorf("failed to get machine %q details: %v", machineId, err)
		return nil, err
	}
	name := machine.Tag()
	return &lxcContainer{
		machine:   machine,
		Container: golxc.New(name),
	}, nil
}

func (lxc *lxcContainer) Create() error {
	// Create the cloud-init.
	directory := lxc.Directory()
	if err := os.MkdirAll(directory, 0755); err != nil {
		logger.Errorf("failed to create container directory: %v", err)
		return err
	}
	// Write the userData to a temp file and use that filename as a start template param
	userData := []byte("#cloud-init\n") // call userData
	userDataFilename := filepath.Join(lxc.Directory(), "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return err
	}
	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", lxc.Name(), // Use the container name as the hostid
		"-r", lxc.machine.Series(),
	}
	// Create the container.
	if err := lxc.Container.Create(defaultTemplate, templateParams...); err != nil {
		logger.Errorf("lxc container creation failed: %v", err)
		return err
	}
	return nil
}

func (lxc *lxcContainer) Start() error {

	// Start the lxc container with the appropriate settings for grabbing the
	// console output and a log file.

	return fmt.Errorf("Not yet implemented")
}

// Defer the Stop and Destroy methods to the composed lxc.Container

func (lxc *lxcContainer) Directory() string {
	return filepath.Join(containerDir, lxc.Name())
}

func (lxc *lxcContainer) userData(nonce string, tools *state.Tools, cfg *config.Config, cons constraints.Value) ([]byte, error) {
	machineConfig := &cloudinit.MachineConfig{
		MachineId:    lxc.machine.Id(),
		MachineNonce: nonce,
		DataDir:      "/var/lib/juju",
		Tools:        tools,
	}
	// TODO(thumper): add mount points for the /var/lib/juju/tools dir and /var/log/juju for the machine logs.
	if err := environs.FinishMachineConfig(machineConfig, cfg, cons); err != nil {
		return nil, err
	}
	cloudConfig, err := cloudinit.New(machineConfig)
	if err != nil {
		return nil, err
	}
	data, err := cloudConfig.Render()
	if err != nil {
		return nil, err
	}
	return data, nil
}
