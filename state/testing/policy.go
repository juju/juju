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
	id                   string
	isManual             bool
	isManualNotSupported bool
}

// NewMachineInfoGetter creates a mock implementing the
// state.MachineInfoGetter interface.
func NewMachineInfoGetter(id string, isManual, isManualNotSupported bool) state.MachineInfoGetter {
	return &mockMachineInfoGetter{
		id:                   id,
		isManual:             isManual,
		isManualNotSupported: isManualNotSupported,
	}
}

// Id returns the identifier of the simulated machine.
func (mig *mockMachineInfoGetter) Id() string {
	return mig.id
}

// IsManual returns whether the simulated machine was manually provisioned.
func (mig *mockMachineInfoGetter) IsManual() (bool, bool) {
	return mig.isManual, mig.isManualNotSupported
}

// RequiresSafeNetworkerTest tests the RequiresSafeNetworker environ capability
// for machine 0 and 1, for each regular and manual provisioning, and for each then
// without and with disabled network management. The same turn is done then with no
// information about manual provisioning due to a lower server API version. In this
// case the safe networker is always required.
//
// So a standard pattern of 16 boolean results is
//
// Machine 0: false, true, true, true,
// Machine 1: false, true, true, true,
// Machine 0: true, true, true, true,
// Machine 1: true, true, true, true,
//
// Only local, maas, and manual provider behave different.
func RequiresSafeNetworkerTest(c *gc.C, env environs.Environ, requirements [16]bool) {
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
		{&mockMachineInfoGetter{"0", false, false}, false},
		{&mockMachineInfoGetter{"0", false, false}, true},
		{&mockMachineInfoGetter{"0", true, false}, false},
		{&mockMachineInfoGetter{"0", true, false}, true},
		{&mockMachineInfoGetter{"1", false, false}, false},
		{&mockMachineInfoGetter{"1", false, false}, true},
		{&mockMachineInfoGetter{"1", true, false}, false},
		{&mockMachineInfoGetter{"1", true, false}, true},
	}
	for i, test := range tests {
		isManual, ok := test.mig.IsManual()
		c.Logf("test %d: machine: %q, is manual: %v, ok: %v, disable networking: %v",
			i, test.mig.Id(), isManual, ok, test.disableNetworkManagement)

		attrs := env.Config().AllAttrs()
		attrs["disable-network-management"] = test.disableNetworkManagement
		newCfg, err := config.New(config.NoDefaults, attrs)
		c.Check(err, gc.IsNil)
		err = env.SetConfig(newCfg)
		c.Check(err, gc.IsNil)
		c.Check(env.RequiresSafeNetworker(test.mig), gc.Equals, requirements[i])
	}
}

// RequiresSafeNetworkerTestDefault represents the the usual case where
// a safe networker is required for disabled network management or
// manual provisioning. Of the latter information isn't available due
// to API V0 it is required too.
var RequiresSafeNetworkerTestDefault = [16]bool{
	// API v1 or higher, machines 0 and 1.
	false, true, true, true,
	false, true, true, true,
	// API v0, machines 0 and 1.
	true, true, true, true,
	true, true, true, true,
}
