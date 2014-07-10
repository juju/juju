// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver"
	"github.com/juju/juju/environs/config"
)

type MockPolicy struct {
	GetPrechecker           func(*config.Config) (environmentserver.Prechecker, error)
	GetConfigValidator      func(string) (environmentserver.ConfigValidator, error)
	GetEnvironCapability    func(*config.Config) (environmentserver.EnvironCapability, error)
	GetConstraintsValidator func(*config.Config) (constraints.Validator, error)
	GetInstanceDistributor  func(*config.Config) (environmentserver.InstanceDistributor, error)
}

func (p *MockPolicy) Prechecker(cfg *config.Config) (environmentserver.Prechecker, error) {
	if p.GetPrechecker != nil {
		return p.GetPrechecker(cfg)
	}
	return nil, errors.NotImplementedf("Prechecker")
}

func (p *MockPolicy) ConfigValidator(providerType string) (environmentserver.ConfigValidator, error) {
	if p.GetConfigValidator != nil {
		return p.GetConfigValidator(providerType)
	}
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (p *MockPolicy) EnvironCapability(cfg *config.Config) (environmentserver.EnvironCapability, error) {
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

func (p *MockPolicy) InstanceDistributor(cfg *config.Config) (environmentserver.InstanceDistributor, error) {
	if p.GetInstanceDistributor != nil {
		return p.GetInstanceDistributor(cfg)
	}
	return nil, errors.NewNotImplemented(nil, "InstanceDistributor")
}
