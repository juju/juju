// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sort"

	"github.com/juju/version"

	"github.com/juju/juju/api/common"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	if ep, ok := p.(*environProvisioner); ok {
		configObserver = &ep.configObserver
		configObserver.catacomb = &ep.catacomb
	} else {
		cp := p.(*containerProvisioner)
		configObserver = &cp.configObserver
		configObserver.catacomb = &cp.catacomb
	}
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}

var (
	GetContainerInitialiser = &getContainerInitialiser
	GetToolsFinder          = &getToolsFinder
	RetryStrategyDelay      = &retryStrategyDelay
	RetryStrategyCount      = &retryStrategyCount
)

var ClassifyMachine = classifyMachine

// GetCopyAvailabilityZoneMachines returns a copy of p.(*provisionerTask).availabilityZoneMachines
func GetCopyAvailabilityZoneMachines(p ProvisionerTask) []AvailabilityZoneMachine {
	task := p.(*provisionerTask)
	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	// sort to make comparisons in the tests easier.
	sort.Sort(azMachineSort(task.availabilityZoneMachines))
	retvalues := make([]AvailabilityZoneMachine, len(task.availabilityZoneMachines))
	for i := range task.availabilityZoneMachines {
		retvalues[i] = *task.availabilityZoneMachines[i]
	}
	return retvalues
}

func SetupToStartMachine(p ProvisionerTask, machine apiprovisioner.MachineProvisioner, version *version.Number) (
	environs.StartInstanceParams,
	error,
) {
	return p.(*provisionerTask).setupToStartMachine(machine, version)
}

func (cs *ContainerSetup) SetGetNetConfig(getNetConf func(common.NetworkConfigSource) ([]params.NetworkConfig, error)) {
	cs.getNetConfig = getNetConf
}
