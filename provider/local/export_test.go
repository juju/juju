// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

var (
	CheckIfRoot        = &checkIfRoot
	CheckLocalPort     = &checkLocalPort
	DetectAptProxies   = &detectAptProxies
	ExecuteCloudConfig = &executeCloudConfig
	Provider           = providerInstance
	UserCurrent        = &userCurrent
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

type mockInstance struct {
	id                string
	instance.Instance // stub out other methods with panics
}

func (inst *mockInstance) Id() instance.Id {
	return instance.Id(inst.id)
}

type startInstanceFunc func(*localEnviron, environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, error)

func PatchCreateContainer(s *testing.CleanupSuite, c *gc.C, expectedURL string) startInstanceFunc {
	mockFunc := func(_ *localEnviron, args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, error) {
		c.Assert(args.Tools, gc.HasLen, 1)
		c.Assert(args.Tools[0].URL, gc.Equals, expectedURL)
		return &mockInstance{id: "mock"}, nil, nil
	}
	s.PatchValue(&createContainer, mockFunc)
	return mockFunc
}
