// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
)

var Provider = provider

// SetRootCheckFunction allows tests to override the check for a root user.
// The return value is the function to restore the old value.
func SetRootCheckFunction(f func() bool) func() {
	old := checkIfRoot
	checkIfRoot = f
	return func() { checkIfRoot = old }
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

func SudoCallerIds() (uid, gid int, err error) {
	return sudoCallerIds()
}
