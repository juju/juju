// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
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
	"launchpad.net/juju-core/state/api"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate = "ubuntu-cloud"
	containerDir    = "/var/lib/juju/containers"
)

type lxcContainer struct {
	*golxc.Container
	machineId     string
	series        string
	nonce         string
	tools         *state.Tools
	environConfig *config.Config
	stateInfo     *state.Info
	apiInfo       *api.Info
	cons          constraints.Value
}

// TODO(thumper): care about constraints...
func NewContainer(
	machineId, series, nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) (container.Container, error) {
	name := state.MachineTag(machineId)
	return &lxcContainer{
		Container:     golxc.New(name),
		machineId:     machineId,
		series:        series,
		nonce:         nonce,
		tools:         tools,
		environConfig: environConfig,
		stateInfo:     stateInfo,
		apiInfo:       apiInfo,
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
	userData, err := lxc.userData()
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return err
	}

	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return err
	}
	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", lxc.Name(), // Use the container name as the hostid
		"-r", lxc.series,
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
	directory := lxc.Directory()
	consoleFile := filepath.Join(directory, "console.log")
	lxc.Container.LogFile = filepath.Join(directory, "container.log")
	lxc.Container.LogLevel = golxc.LogDebug
	// configFile needed maybe later for ipconfig and mount points
	configFile := ""
	return lxc.Container.Start(configFile, consoleFile)
}

// Defer the Stop and Destroy methods to the composed lxc.Container

// TODO: Destroy should also remove the directory... (or rename it and save it for later analysis)

func (lxc *lxcContainer) Directory() string {
	return filepath.Join(containerDir, lxc.Name())
}

func (lxc *lxcContainer) userData() ([]byte, error) {
	machineConfig := &cloudinit.MachineConfig{
		MachineId:    lxc.machineId,
		MachineNonce: lxc.nonce,
		StateInfo:    lxc.stateInfo,
		APIInfo:      lxc.apiInfo,
		DataDir:      "/var/lib/juju",
		Tools:        lxc.tools,
	}
	// TODO(thumper): add mount points for the /var/lib/juju/tools dir and /var/log/juju for the machine logs.
	if err := environs.FinishMachineConfig(machineConfig, lxc.environConfig, lxc.cons); err != nil {
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
