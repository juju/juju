// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/golxc"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate     = "ubuntu-cloud"
	containerDir        = "/var/lib/juju/containers"
	removedContainerDir = "/var/lib/juju/removed-containers"
	lxcContainerDir     = "/var/lib/lxc"
	lxcObjectFactory    = golxc.Factory()
)

// ManagerConfig contains the initialization parameters for the ContainerManager.
type ManagerConfig struct {
	Name   string
	LogDir string
}

// ContainerManager is responsible for starting containers, and stopping and
// listing containers that it has started.  The name of the manager is used to
// namespace the lxc containers on the machine.
type ContainerManager interface {
	// StartContainer creates and starts a new lxc container for the specified machine.
	StartContainer(
		machineId, series, nonce string,
		tools *state.Tools,
		environConfig *config.Config,
		stateInfo *state.Info,
		apiInfo *api.Info) (instance.Instance, error)
	// StopContainer stops and destroyes the lxc container identified by Instance.
	StopContainer(instance.Instance) error
	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instance.Instance, error)
}

type containerManager struct {
	name   string
	logdir string
}

// NewContainerManager returns a manager object that can start and stop lxc
// containers. The containers that are created are namespaced by the name
// parameter.
func NewContainerManager(conf ManagerConfig) ContainerManager {
	logdir := "/var/log/juju"
	if conf.LogDir != "" {
		logdir = conf.LogDir
	}
	return &containerManager{name: conf.Name, logdir: logdir}
}

func (manager *containerManager) StartContainer(
	machineId, series, nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info) (instance.Instance, error) {

	name := state.MachineTag(machineId)
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}
	// Note here that the lxcObjectFacotry only returns a valid container
	// object, and doesn't actually construct the underlying lxc container on
	// disk.
	container := lxcObjectFactory.New(name)

	// Create the cloud-init.
	directory := jujuContainerDirectory(name)
	logger.Tracef("create directory: %s", directory)
	if err := os.MkdirAll(directory, 0755); err != nil {
		logger.Errorf("failed to create container directory: %v", err)
		return nil, err
	}
	logger.Tracef("write cloud-init")
	userDataFilename, err := writeUserData(directory, machineId, nonce, tools, environConfig, stateInfo, apiInfo)
	if err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return nil, err
	}
	logger.Tracef("write the lxc.conf file")
	configFile, err := writeLxcConfig(directory, manager.logdir)
	if err != nil {
		logger.Errorf("failed to write config file: %v", err)
		return nil, err
	}
	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", name, // Use the container name as the hostid
		"-r", series,
	}
	// Create the container.
	logger.Tracef("create the container")
	if err := container.Create(configFile, defaultTemplate, templateParams...); err != nil {
		logger.Errorf("lxc container creation failed: %v", err)
		return nil, err
	}
	// Make sure that the mount dir has been created.
	logger.Tracef("make the mount dir for the shard logs")
	if err := os.MkdirAll(internalLogDir(name), 0755); err != nil {
		logger.Errorf("failed to create internal /var/log/juju mount dir: %v", err)
		return nil, err
	}
	logger.Tracef("lxc container created")

	// Start the lxc container with the appropriate settings for grabbing the
	// console output and a log file.
	consoleFile := filepath.Join(directory, "console.log")
	container.SetLogFile(filepath.Join(directory, "container.log"), golxc.LogDebug)
	logger.Tracef("start the container")
	// We explicitly don't pass through the config file to the container.Start
	// method as we have passed it through at container creation time.  This
	// is necessary to get the appropriate rootfs reference without explicitly
	// setting it ourselves.
	if err = container.Start("", consoleFile); err != nil {
		logger.Errorf("container failed to start: %v", err)
		return nil, err
	}
	logger.Tracef("container started")
	return &lxcInstance{name}, nil
}

func (manager *containerManager) StopContainer(instance instance.Instance) error {
	name := string(instance.Id())
	container := lxcObjectFactory.New(name)
	if err := container.Stop(); err != nil {
		logger.Errorf("failed to stop lxc container: %v", err)
		return err
	}
	if err := container.Destroy(); err != nil {
		logger.Errorf("failed to destroy lxc container: %v", err)
		return err
	}
	// Move the directory.
	logger.Tracef("create old container dir: %s", removedContainerDir)
	if err := os.MkdirAll(removedContainerDir, 0755); err != nil {
		logger.Errorf("failed to create removed container directory: %v", err)
		return err
	}
	removedDir, err := uniqueDirectory(removedContainerDir, name)
	if err != nil {
		logger.Errorf("was not able to generate a unique directory: %v", err)
		return err
	}
	if err := os.Rename(jujuContainerDirectory(name), removedDir); err != nil {
		logger.Errorf("failed to rename container directory: %v", err)
		return err
	}
	return nil
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	containers, err := lxcObjectFactory.List()
	if err != nil {
		logger.Errorf("failed getting all instances: %v", err)
		return
	}
	managerPrefix := ""
	if manager.name != "" {
		managerPrefix = fmt.Sprintf("%s-", manager.name)
	}

	for _, container := range containers {
		// Filter out those not starting with our name.
		name := container.Name()
		if !strings.HasPrefix(name, managerPrefix) {
			continue
		}
		if container.IsRunning() {
			result = append(result, &lxcInstance{name})
		}
	}
	return
}

func jujuContainerDirectory(containerName string) string {
	return filepath.Join(containerDir, containerName)
}

const internalLogDirTemplate = "%s/%s/rootfs/var/log/juju"

func internalLogDir(containerName string) string {
	return fmt.Sprintf(internalLogDirTemplate, lxcContainerDir, containerName)
}

const localConfig = `
lxc.network.type = veth
lxc.network.link = lxcbr0
lxc.network.flags = up

lxc.mount.entry=%s var/log/juju none defaults,bind 0 0
`

func writeLxcConfig(directory, logdir string) (string, error) {
	// TODO(thumper): support different network settings.
	configFilename := filepath.Join(directory, "lxc.conf")
	configContent := fmt.Sprintf(localConfig, logdir)
	if err := ioutil.WriteFile(configFilename, []byte(configContent), 0644); err != nil {
		return "", err
	}
	return configFilename, nil
}

func writeUserData(
	directory, machineId, nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) (string, error) {
	userData, err := cloudInitUserData(machineId, nonce, tools, environConfig, stateInfo, apiInfo)
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

func cloudInitUserData(
	machineId, nonce string,
	tools *state.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info,
) ([]byte, error) {
	machineConfig := &cloudinit.MachineConfig{
		MachineId:            machineId,
		MachineNonce:         nonce,
		MachineContainerType: instance.LXC,
		StateInfo:            stateInfo,
		APIInfo:              apiInfo,
		DataDir:              "/var/lib/juju",
		Tools:                tools,
	}
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

// uniqueDirectory returns "path/name" if that directory doesn't exist.  If it
// does, the method starts appending .1, .2, etc until a unique name is found.
func uniqueDirectory(path, name string) (string, error) {
	dir := filepath.Join(path, name)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return dir, nil
	}
	for i := 1; ; i++ {
		dir := filepath.Join(path, fmt.Sprintf("%s.%d", name, i))
		_, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return dir, nil
		} else if err != nil {
			return "", err
		}
	}
	panic("unreachable")
}
