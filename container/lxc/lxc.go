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
	machineId string
}

// TODO(thumper): care about constraints...
func NewContainer(machineId string) (container.Container, error) {
	name := state.MachineTag(machineId)
	return &lxcContainer{
		Container: golxc.New(name),
		machineId: machineId,
	}, nil
}

func (lxc *lxcContainer) Create(
	series, nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) error {
	// Create the cloud-init.
	directory := lxc.Directory()
	if err := os.MkdirAll(directory, 0755); err != nil {
		logger.Errorf("failed to create container directory: %v", err)
		return err
	}
	if err := lxc.WriteConfig(); err != nil {
		logger.Errorf("failed to write config file: %v", err)
		return err
	}
	userDataFilename, err := lxc.WriteUserData(nonce, tools, environConfig, stateInfo, apiInfo)
	if err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return err
	}
	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", lxc.Name(), // Use the container name as the hostid
		"-r", series,
	}
	// Create the container.
	if err := lxc.Container.Create(defaultTemplate, templateParams...); err != nil {
		logger.Errorf("lxc container creation failed: %v", err)
		return err
	}
	// Make sure that the mount dir has been created.
	if err := os.MkdirAll(lxc.InternalLogDir(), 0755); err != nil {
		logger.Errorf("failed to create internal /var/log/juju mount dir: %v", err)
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

const internalLogDir = "/var/lib/lxc/%s/rootfs/var/log/juju"

func (lxc *lxcContainer) InternalLogDir() string {
	return fmt.Sprintf(internalLogDir, lxc.Name())
}

const localConfig = `
lxc.network.type = veth
lxc.network.link = lxcbr0
lxc.network.flags = up

lxc.mount.entry=/var/log/juju %s none defaults,bind 0 0
`

func (lxc *lxcContainer) WriteConfig() error {
	// TODO(thumper): support different network settings.
	config := fmt.Sprintf(localConfig, lxc.InternalLogDir())
	configFilename := filepath.Join(lxc.Directory(), "lxc.conf")
	if err := ioutil.WriteFile(configFilename, []byte(config), 0644); err != nil {
		return err
	}
	lxc.Container.ConfigFile = configFilename
	return nil
}

func (lxc *lxcContainer) WriteUserData(
	nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) (string, error) {
	userData, err := lxc.userData(nonce, tools, environConfig, stateInfo, apiInfo)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	userDataFilename := filepath.Join(lxc.Directory(), "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

func (lxc *lxcContainer) userData(
	nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) ([]byte, error) {
	machineConfig := &cloudinit.MachineConfig{
		MachineId:            lxc.machineId,
		MachineNonce:         nonce,
		MachineContainerType: "lxc",
		StateInfo:            stateInfo,
		APIInfo:              apiInfo,
		DataDir:              "/var/lib/juju",
		Tools:                tools,
	}
	// TODO(thumper): add mount points for the /var/lib/juju/tools dir and /var/log/juju for the machine logs.
	if err := environs.FinishMachineConfig(machineConfig, environConfig, constraints.Value{}); err != nil {
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
