// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
)

var (
	CheckIfRoot      = &checkIfRoot
	CheckLocalPort   = &checkLocalPort
	DetectAptProxies = &detectAptProxies
	FinishBootstrap  = &finishBootstrap
	Provider         = providerInstance
	UserCurrent      = &userCurrent
)

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
	return testing.PatchValue(&getAddressForInterface, func(name string) (string, error) {
		logger.Debugf("getAddressForInterface called for %s", name)
		return "127.0.0.1", nil
	})
}
