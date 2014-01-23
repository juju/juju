// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"sync"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/state/api/watcher"
)

var (
	proxy      osenv.ProxySettings
	proxyMutex sync.Mutex
)

type Facade interface {
	WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error)
	EnvironConfig() (*config.Config, error)
}

// UpdatePackageProxy updates the package proxy settings from the
// environment.
func UpdatePackageProxy(cfg *config.Config) {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()

	newSettings := cfg.ProxySettings()
	if proxy != newSettings {
		proxy = newSettings
		logger.Debugf("Updated proxy settings: %#v", proxy)
	}
}
