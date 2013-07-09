// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
)

var Provider = provider

// SetDefaultRootDir overrides the default root directory for testing
// purposes.
func SetDefaultRootDir(rootdir string) (old string) {
	old, defaultRootDir = defaultRootDir, rootdir
	return
}

// SetRootCheckFunction allows tests to override the check for a root user.
func SetRootCheckFunction(f func() bool) (old func() bool) {
	old, rootCheckFunction = rootCheckFunction, f
	return
}

// SetUpstartScriptLocation allows tests to override the directory where the
// provider writes the upstart scripts.
func SetUpstartScriptLocation(location string) (old string) {
	old, upstartScriptLocation = upstartScriptLocation, location
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

func GetSudoCallerIds() (uid, gid int, err error) {
	return getSudoCallerIds()
}
