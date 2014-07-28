// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver"
	"github.com/juju/juju/environs/config"
)

type MockEnvironmentValidator struct {
	GetValidate              func(newConfig, oldConfig *config.Config) (*config.Config, error)
	GetValidateConstraints   func(constraints.Value) ([]string, error)
	GetSupportsUnitPlacement func() error
	GetResolveConstraints    func(constraints.Value) (constraints.Value, error)
	GetConstraintsValidator  func() (constraints.Validator, error)
}

func (p *MockEnvironmentValidator) ValidateConstraints(cons constraints.Value) ([]string, error) {
	if p.GetValidateConstraints != nil {
		return p.GetValidateConstraints(cons)
	}
	return nil, errors.NotImplementedf("ValidateConstraints")
}

func (p *MockEnvironmentValidator) Validate(newConfig, oldConfig *config.Config) (*config.Config, error) {
	if p.GetValidate != nil {
		return p.GetValidate(newConfig, oldConfig)
	}
	return nil, errors.NotImplementedf("Validate")
}

func (p *MockEnvironmentValidator) SupportsUnitPlacement() error {
	if p.GetSupportsUnitPlacement() != nil {
		return p.GetSupportsUnitPlacement()
	}
	return errors.NotImplementedf("SupportsUnitPlacement")
}

func (p *MockEnvironmentValidator) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	if p.GetResolveConstraints != nil {
		return p.GetResolveConstraints(cons)
	}
	return constraints.Value{}, errors.NotImplementedf("ResolveConstraints")
}

func (p *MockEnvironmentValidator) ConstraintsValidator() (constraints.Validator, error) {
	if p.GetConstraintsValidator != nil {
		return p.GetConstraintsValidator()
	}
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

type MockInstanceDistributor struct {
	GetInstanceDistributor func(*config.Config) (environmentserver.InstanceDistributor, error)
}

func (p *MockInstanceDistributor) InstanceDistributor(cfg *config.Config) (environmentserver.InstanceDistributor, error) {
	if p.GetInstanceDistributor != nil {
		return p.GetInstanceDistributor(cfg)
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}
