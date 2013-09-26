// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"os/exec"
	"strings"

	"launchpad.net/loggo"

	base "launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
)

var (
	logger = loggo.GetLogger("juju.container.kvm")

	kvmObjectFactory base.ContainerFactory = &containerFactory{}

	containerDir        = "/var/lib/juju/containers"
	removedContainerDir = "/var/lib/juju/removed-containers"
)

// IsKVMSupported calls into the kvm-ok executable from the cpu-checkers package.
// It is a variable to allow us to overrid behaviour in the tests.
var IsKVMSupported = func() bool {
	command := exec.Command("kvm-ok")
	output, err := command.CombinedOutput()
	if err == exec.ErrNotFound {
		logger.Warningf("kvm-ok command not found")
		return false
	} else if err != nil {
		logger.Errorf("%v", err)
		return false
	}
	logger.Debugf("kvm-ok output:\n%s", output)
	return command.ProcessState.Success()
}

// ContainerManager is responsible for starting containers, and stopping and
// listing containers that it has started.  The name of the manager is used to
// namespace the kvm containers on the machine.
type ContainerManager interface {
	// StartContainer creates and starts a new kvm container for the specified machine.
	StartContainer(
		machineId, series, nonce string,
		tools *tools.Tools,
		environConfig *config.Config,
		stateInfo *state.Info,
		apiInfo *api.Info) (instance.Instance, error)
	// StopContainer stops and destroyes the kvm container identified by Instance.
	StopContainer(instance.Instance) error
	// ListContainers return a list of containers that have been started by
	// this manager.
	ListContainers() ([]instance.Instance, error)
}

// NewContainerManager returns a manager object that can start and stop kvm
// containers. The containers that are created are namespaced by the name
// parameter.
func NewContainerManager(name string) (ContainerManager, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return &containerManager{name: name}, nil
}

// containerManager handles all of the business logic at the juju specific
// level. It makes sure that the necessary directories are in place, that the
// user-data is written out in the right place.
type containerManager struct {
	name string
}

var _ ContainerManager = (*containerManager)(nil)

func (manager *containerManager) StartContainer(
	machineId, series, nonce string,
	tools *tools.Tools,
	environConfig *config.Config,
	stateInfo *state.Info,
	apiInfo *api.Info) (instance.Instance, error) {
	return nil, fmt.Errorf("not yet implemented")
}

func (manager *containerManager) StopContainer(instance.Instance) error {
	return fmt.Errorf("not yet implemented")
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	containers, err := kvmObjectFactory.List()
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
