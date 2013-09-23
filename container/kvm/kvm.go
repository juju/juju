// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"os/exec"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.container.kvm")

// ManagerConfig contains the initialization parameters for the ContainerManager.
type ManagerConfig struct {
	Name   string
	LogDir string
}

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
// namespace the lxc containers on the machine.
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

// containerManager handles all of the business logic at the juju specific
// level. It makes sure that the necessary directories are in place, that the
// user-data is written out in the right place.
type containerManager struct {
	name   string
	logDir string
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

func (manager *containerManager) ListContainers() ([]instance.Instance, error) {
	return nil, fmt.Errorf("not yet implemented")
}
