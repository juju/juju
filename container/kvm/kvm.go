// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"os/exec"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
)

var (
	logger = loggo.GetLogger("juju.container.kvm")

	KvmObjectFactory ContainerFactory = &containerFactory{}
)

// IsKVMSupported calls into the kvm-ok executable from the cpu-checkers package.
// It is a variable to allow us to overrid behaviour in the tests.
var IsKVMSupported = func() (bool, error) {
	command := exec.Command("kvm-ok")
	output, err := command.CombinedOutput()
	if err != nil {
		return false, err
	}
	logger.Debugf("kvm-ok output:\n%s", output)
	return command.ProcessState.Success(), nil
}

// NewContainerManager returns a manager object that can start and stop kvm
// containers. The containers that are created are namespaced by the name
// parameter.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	if conf.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if conf.LogDir == "" {
		conf.LogDir = "/var/log/juju"
	}
	return &containerManager{name: conf.Name, logdir: conf.LogDir}, nil
}

// containerManager handles all of the business logic at the juju specific
// level. It makes sure that the necessary directories are in place, that the
// user-data is written out in the right place.
type containerManager struct {
	name   string
	logdir string
}

var _ container.Manager = (*containerManager)(nil)

func (manager *containerManager) StartContainer(
	machineConfig *cloudinit.MachineConfig,
	series string,
	network *container.NetworkConfig) (instance.Instance, error) {
	return nil, fmt.Errorf("not yet implemented")
}

func (manager *containerManager) StopContainer(instance.Instance) error {
	return fmt.Errorf("not yet implemented")
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	containers, err := KvmObjectFactory.List()
	if err != nil {
		logger.Errorf("failed getting all instances: %v", err)
		return
	}
	managerPrefix := fmt.Sprintf("%s-", manager.name)
	for _, container := range containers {
		// Filter out those not starting with our name.
		name := container.Name()
		if !strings.HasPrefix(name, managerPrefix) {
			continue
		}
		if container.IsRunning() {
			result = append(result, &kvmInstance{container, name})
		}
	}
	return
}
