// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type MockPolicy struct {
	GetPrechecker              func() (state.Prechecker, error)
	GetConfigValidator         func() (config.Validator, error)
	GetConstraintsValidator    func() (constraints.Validator, error)
	GetInstanceDistributor     func() (instance.Distributor, error)
	GetStorageProviderRegistry func() (storage.ProviderRegistry, error)
}

func (p *MockPolicy) Prechecker() (state.Prechecker, error) {
	if p.GetPrechecker != nil {
		return p.GetPrechecker()
	}
	return nil, errors.NotImplementedf("Prechecker")
}

func (p *MockPolicy) ConfigValidator() (config.Validator, error) {
	if p.GetConfigValidator != nil {
		return p.GetConfigValidator()
	}
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (p *MockPolicy) ConstraintsValidator() (constraints.Validator, error) {
	if p.GetConstraintsValidator != nil {
		return p.GetConstraintsValidator()
	}
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (p *MockPolicy) InstanceDistributor() (instance.Distributor, error) {
	if p.GetInstanceDistributor != nil {
		return p.GetInstanceDistributor()
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}

func (p *MockPolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	if p.GetStorageProviderRegistry != nil {
		return p.GetStorageProviderRegistry()
	}
	return nil, errors.NotImplementedf("StorageProviderRegistry")
}
