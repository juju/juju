// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"launchpad.net/juju-core/environs/config"
)

var Provider = provider

// SetDefaultRootDir overrides the default root directory for testing
// purposes.
func SetDefaultRootDir(rootdir string) (old string) {
	old, defaultRootDir = defaultRootDir, rootdir
	return
}

// ConfigNamespace returns the result of the namespace call on the
// localConfig.
func ConfigNamespace(cfg *config.Config) string {
	localConfig, _ := provider.newConfig(cfg)
	return localConfig.namespace()
}

// CreateDirs calls createDirs on the localEnviron.
func CreateDirs(cfg *config.Config) error {
	localConfig, _ := provider.newConfig(cfg)
	return localConfig.createDirs()
}
