// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
)

var Provider = provider

// SetDefaultRootDir overrides the default root directory for testing
// purposes.
func SetDefaultRootDir(rootdir string) (old string) {
	old, environs.DataDir = environs.DataDir, rootdir
	return
}

// ConfigNamespace returns the result of the namespace call on the
// localConfig.
func ConfigNamespace(cfg *config.Config) string {
	localConfig, _ := provider.newConfig(cfg)
	return localConfig.namespace()
}

// CreateDirs calls createDirs on the localEnviron.
func CreateDirs(c *gc.C, cfg *config.Config) error {
	localConfig, err := provider.newConfig(cfg)
	c.Assert(err, gc.IsNil)
	return localConfig.createDirs()
}

// CheckDirs returns the list of directories to check for permissions in the test.
func CheckDirs(c *gc.C, cfg *config.Config) []string {
	localConfig, err := provider.newConfig(cfg)
	c.Assert(err, gc.IsNil)
	return []string{
		localConfig.rootDir(),
		localConfig.sharedStorageDir(),
		localConfig.storageDir(),
		localConfig.mongoDir(),
	}
}
