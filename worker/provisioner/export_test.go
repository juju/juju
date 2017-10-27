// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sort"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	if ep, ok := p.(*environProvisioner); ok {
		configObserver = &ep.configObserver
	} else {
		cp := p.(*containerProvisioner)
		configObserver = &cp.configObserver
	}
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}

var (
	ContainerManagerConfig   = containerManagerConfig
	GetContainerInitialiser  = &getContainerInitialiser
	GetToolsFinder           = &getToolsFinder
	ResolvConf               = &resolvConf
	RetryStrategyDelay       = &retryStrategyDelay
	RetryStrategyCount       = &retryStrategyCount
	GetObservedNetworkConfig = &getObservedNetworkConfig
)

var ClassifyMachine = classifyMachine

// GetCopyAvailabilityZoneMachines returns a copy of p.(*provisionerTask).availabilityZoneMachines
func GetCopyAvailabilityZoneMachines(p ProvisionerTask) []AvailabilityZoneMachine {
	task := p.(*provisionerTask)
	task.azMachinesMutex.RLock()
	defer task.azMachinesMutex.RUnlock()
	// sort to make comparisions in the tests easier.
	sort.Sort(byPopulationThenNames(task.availabilityZoneMachines))
	retvalues := make([]AvailabilityZoneMachine, len(task.availabilityZoneMachines))
	for i, _ := range task.availabilityZoneMachines {
		retvalues[i] = *task.availabilityZoneMachines[i]
	}
	return retvalues
}
