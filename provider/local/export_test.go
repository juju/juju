// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing/testbase"
)

var (
	CheckLocalPort         = &checkLocalPort
	DetectAptProxies       = &detectAptProxies
	EnvKeyTestingForceSlow = envKeyTestingForceSlow
	FinishBootstrap        = &finishBootstrap
	Provider               = providerInstance
	ReleaseVersion         = &releaseVersion
	UseFastLXC             = useFastLXC
	UserCurrent            = &userCurrent
)

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
	env, _ := providerInstance.Open(cfg)
	return env.(*localEnviron).config.namespace()
}

// CreateDirs calls createDirs on the localEnviron.
func CreateDirs(c *gc.C, cfg *config.Config) error {
	env, err := providerInstance.Open(cfg)
	c.Assert(err, gc.IsNil)
	return env.(*localEnviron).config.createDirs()
}

// CheckDirs returns the list of directories to check for permissions in the test.
func CheckDirs(c *gc.C, cfg *config.Config) []string {
	localConfig, err := providerInstance.newConfig(cfg)
	c.Assert(err, gc.IsNil)
	return []string{
		localConfig.rootDir(),
		localConfig.storageDir(),
		localConfig.mongoDir(),
	}
}

// MockAddressForInterface replaces the getAddressForInterface with a function
// that returns a constant localhost ip address.
func MockAddressForInterface() func() {
	return testbase.PatchValue(&getAddressForInterface, func(name string) (string, error) {
		logger.Debugf("getAddressForInterface called for %s", name)
		return "127.0.0.1", nil
	})
}
