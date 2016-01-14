// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct{}

var _ state.Policy = environStatePolicy{}

// NewStatePolicy returns a state.Policy that is
// implemented in terms of Environ and related
// types.
func NewStatePolicy() state.Policy {
	return environStatePolicy{}
}

func (environStatePolicy) Prechecker(cfg *config.Config) (state.Prechecker, error) {
	// Environ implements state.Prechecker.
	return New(cfg)
}

func (environStatePolicy) ConfigValidator(providerType string) (state.ConfigValidator, error) {
	// EnvironProvider implements state.ConfigValidator.
	return Provider(providerType)
}

func (environStatePolicy) EnvironCapability(cfg *config.Config) (state.EnvironCapability, error) {
	// Environ implements state.EnvironCapability.
	return New(cfg)
}

func (environStatePolicy) ConstraintsValidator(cfg *config.Config, querier state.SupportedArchitecturesQuerier) (constraints.Validator, error) {
	env, err := New(cfg)
	if err != nil {
		return nil, err
	}

	// Ensure that supported architectures are filtered based on cloud specification.
	// TODO (anastasiamac 2015-12-22) this cries for a test \o/
	region := ""
	if cloudEnv, ok := env.(simplestreams.HasRegion); ok {
		cloudCfg, err := cloudEnv.Region()
		if err != nil {
			return nil, err
		}
		region = cloudCfg.Region
	}
	arches, err := querier.SupportedArchitectures(env.Config().AgentStream(), region)
	if err != nil {
		return nil, err
	}

	// Construct provider specific validator.
	val, err := env.ConstraintsValidator()
	if err != nil {
		return nil, err
	}

	// Update validator architectures with supported architectures from stored
	// cloud image metadata.
	if len(arches) != 0 {
		val.UpdateVocabulary(constraints.Arch, arches)
	}
	return val, nil
}

func (environStatePolicy) InstanceDistributor(cfg *config.Config) (state.InstanceDistributor, error) {
	env, err := New(cfg)
	if err != nil {
		return nil, err
	}
	if p, ok := env.(state.InstanceDistributor); ok {
		return p, nil
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}
