// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type MockPolicy struct {
	GetPrechecker           func(*config.Config) (state.Prechecker, error)
	GetConfigValidator      func(string) (state.ConfigValidator, error)
	GetEnvironCapability    func(*config.Config) (state.EnvironCapability, error)
	GetConstraintsValidator func(*config.Config) (constraints.Validator, error)
	GetInstanceDistributor  func(*config.Config) (state.InstanceDistributor, error)
}

func (p *MockPolicy) Prechecker(cfg *config.Config) (state.Prechecker, error) {
	if p.GetPrechecker != nil {
		return p.GetPrechecker(cfg)
	}
	return nil, errors.NotImplementedf("Prechecker")
}

func (p *MockPolicy) ConfigValidator(providerType string) (state.ConfigValidator, error) {
	if p.GetConfigValidator != nil {
		return p.GetConfigValidator(providerType)
	}
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (p *MockPolicy) EnvironCapability(cfg *config.Config) (state.EnvironCapability, error) {
	if p.GetEnvironCapability != nil {
		return p.GetEnvironCapability(cfg)
	}
	return nil, errors.NotImplementedf("EnvironCapability")
}

func (p *MockPolicy) ConstraintsValidator(cfg *config.Config) (constraints.Validator, error) {
	if p.GetConstraintsValidator != nil {
		return p.GetConstraintsValidator(cfg)
	}
	return nil, errors.NewNotImplemented(nil, "ConstraintsValidator")
}

func (p *MockPolicy) InstanceDistributor(cfg *config.Config) (state.InstanceDistributor, error) {
	if p.GetInstanceDistributor != nil {
		return p.GetInstanceDistributor(cfg)
	}
	return nil, errors.NewNotImplemented(nil, "InstanceDistributor")
}

// mockMachineInfoGetter helps to test the RequiresSafeNetworker
// environment capability.
type mockMachineInfoGetter struct {
	id                  string
	isManual            bool
	isManualNotProvided bool
}

// NewMachineInfoGetter creates a mock implementing the
// state.MachineInfoGetter interface.
func NewMachineInfoGetter(id string, isManual, isManualNotProvided bool) state.MachineInfoGetter {
	return &mockMachineInfoGetter{
		id:                  id,
		isManual:            isManual,
		isManualNotProvided: isManualNotProvided,
	}
}

// Id returns the identifier of the simulated machine.
func (mig *mockMachineInfoGetter) Id() string {
	return mig.id
}

// IsManual returns the manually provisioning flag of the simulated machine.
func (mig *mockMachineInfoGetter) IsManual() (bool, bool) {
	return mig.isManual, mig.isManualNotProvided
}

// CommonRequiresSafeNetworkerTest tests the RequiresSafeNetworker environ capability
// for machine 0 and 1, for each regular and manual provisioning, and for
// each then without and with disabled network management. So eight
// boolean values have to be past.
//
// So a standard pattern is
//
// Machine 0: false, true, true, true,
// Machine 1: false, true, true, true,
//
// Only local, maas, and manual provider behave different.
func CommonRequiresSafeNetworkerTest(c *gc.C, env environs.Environ, requirements [8]bool) {
	tests := []struct {
		mig                      state.MachineInfoGetter
		disableNetworkManagement bool
	}{
		{&mockMachineInfoGetter{"0", false, true}, false},
		{&mockMachineInfoGetter{"0", false, true}, true},
		{&mockMachineInfoGetter{"0", true, true}, false},
		{&mockMachineInfoGetter{"0", true, true}, true},
		{&mockMachineInfoGetter{"1", false, true}, false},
		{&mockMachineInfoGetter{"1", false, true}, true},
		{&mockMachineInfoGetter{"1", true, true}, false},
		{&mockMachineInfoGetter{"1", true, true}, true},
	}
	for i, test := range tests {
		isManual, ok := test.mig.IsManual()
		c.Logf("test %d: machine: %q, is manual: %v, ok: %v, disable networking: %v",
			i, test.mig.Id(), isManual, ok, test.disableNetworkManagement)
		// TODO(mue) Set the disableNetworkManager flag.
		attrs := env.Config().AllAttrs()
		attrs["disable-network-management"] = test.disableNetworkManagement
		newCfg, err := config.New(config.NoDefaults, attrs)
		c.Check(err, gc.IsNil)
		err = env.SetConfig(newCfg)
		c.Check(err, gc.IsNil)
		c.Check(env.RequiresSafeNetworker(test.mig), gc.Equals, requirements[i])
	}
}
