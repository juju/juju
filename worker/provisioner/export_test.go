// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
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
	ContainerManagerConfig  = containerManagerConfig
	GetContainerInitialiser = &getContainerInitialiser
	GetToolsFinder          = &getToolsFinder
	ResolvConf              = &resolvConf
	RetryStrategyDelay      = &retryStrategyDelay
	RetryStrategyCount      = &retryStrategyCount
)

var ClassifyMachine = classifyMachine
