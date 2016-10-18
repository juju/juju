// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/storage"
)

// NewPolicyFunc is the type of a function that,
// given a *State, returns a Policy for that State.
type NewPolicyFunc func(*State) Policy

// Policy is an interface provided to State that may
// be consulted by State to validate or modify the
// behaviour of certain operations.
//
// If a Policy implementation does not implement one
// of the methods, it must return an error that
// satisfies errors.IsNotImplemented, and will thus
// be ignored. Any other error will cause an error
// in the use of the policy.
type Policy interface {
	// Prechecker returns a Prechecker or an error.
	Prechecker() (Prechecker, error)

	// ProviderConfigSchemaSource returns a config.ConfigSchemaSource
	// for the environ provider, or an error.
	ProviderConfigSchemaSource() (config.ConfigSchemaSource, error)

	// ConfigValidator returns a config.Validator or an error.
	ConfigValidator() (config.Validator, error)

	// ConstraintsValidator returns a constraints.Validator or an error.
	ConstraintsValidator() (constraints.Validator, error)

	// InstanceDistributor returns an instance.Distributor or an error.
	InstanceDistributor() (instance.Distributor, error)

	// StorageProviderRegistry returns a storage.ProviderRegistry or an error.
	StorageProviderRegistry() (storage.ProviderRegistry, error)
}

// Prechecker is a policy interface that is provided to State
// to perform pre-flight checking of instance creation.
type Prechecker interface {
	// PrecheckInstance performs a preflight check on the specified
	// series and constraints, ensuring that they are possibly valid for
	// creating an instance in this model.
	//
	// PrecheckInstance is best effort, and not guaranteed to eliminate
	// all invalid parameters. If PrecheckInstance returns nil, it is not
	// guaranteed that the constraints are valid; if a non-nil error is
	// returned, then the constraints are definitely invalid.
	PrecheckInstance(series string, cons constraints.Value, placement string) error
}

// precheckInstance calls the state's assigned policy, if non-nil, to obtain
// a Prechecker, and calls PrecheckInstance if a non-nil Prechecker is returned.
func (st *State) precheckInstance(series string, cons constraints.Value, placement string) error {
	if st.policy == nil {
		return nil
	}
	prechecker, err := st.policy.Prechecker()
	if errors.IsNotImplemented(err) {
		return nil
	} else if err != nil {
		return err
	}
	if prechecker == nil {
		return errors.New("policy returned nil prechecker without an error")
	}
	return prechecker.PrecheckInstance(series, cons, placement)
}

func (st *State) constraintsValidator() (constraints.Validator, error) {
	// Default behaviour is to simply use a standard validator with
	// no model specific behaviour built in.
	var validator constraints.Validator
	if st.policy != nil {
		var err error
		validator, err = st.policy.ConstraintsValidator()
		if errors.IsNotImplemented(err) {
			validator = constraints.NewValidator()
		} else if err != nil {
			return nil, err
		} else if validator == nil {
			return nil, errors.New("policy returned nil constraints validator without an error")
		}
	} else {
		validator = constraints.NewValidator()
	}

	// Add supported architectures gleaned from cloud image
	// metadata to the validator's vocabulary.
	model, err := st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	if region := model.CloudRegion(); region != "" {
		cfg, err := st.ModelConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}
		arches, err := st.CloudImageMetadataStorage.SupportedArchitectures(
			cloudimagemetadata.MetadataFilter{
				Stream: cfg.AgentStream(),
				Region: region,
			},
		)
		if err != nil {
			return nil, errors.Annotate(err, "querying supported architectures")
		}
		if len(arches) != 0 {
			validator.UpdateVocabulary(constraints.Arch, arches)
		}
	}
	return validator, nil
}

// resolveConstraints combines the given constraints with the environ constraints to get
// a constraints which will be used to create a new instance.
func (st *State) resolveConstraints(cons constraints.Value) (constraints.Value, error) {
	validator, err := st.constraintsValidator()
	if err != nil {
		return constraints.Value{}, err
	}
	envCons, err := st.ModelConstraints()
	if err != nil {
		return constraints.Value{}, err
	}
	return validator.Merge(envCons, cons)
}

// validateConstraints returns an error if the given constraints are not valid for the
// current model, and also any unsupported attributes.
func (st *State) validateConstraints(cons constraints.Value) ([]string, error) {
	validator, err := st.constraintsValidator()
	if err != nil {
		return nil, err
	}
	return validator.Validate(cons)
}

// validate calls the state's assigned policy, if non-nil, to obtain
// a config.Validator, and calls Validate if a non-nil config.Validator is
// returned.
func (st *State) validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if st.policy == nil {
		return cfg, nil
	}
	configValidator, err := st.policy.ConfigValidator()
	if errors.IsNotImplemented(err) {
		return cfg, nil
	} else if err != nil {
		return nil, err
	}
	if configValidator == nil {
		return nil, errors.New("policy returned nil configValidator without an error")
	}
	return configValidator.Validate(cfg, old)
}

func (st *State) storageProviderRegistry() (storage.ProviderRegistry, error) {
	if st.policy == nil {
		return storage.StaticProviderRegistry{}, nil
	}
	return st.policy.StorageProviderRegistry()
}

func (st *State) environsProviderConfigSchemaSource() (config.ConfigSchemaSource, error) {
	if st.policy == nil {
		return nil, errors.NotImplementedf("config.ProviderConfigSchemaSource")
	}
	return st.policy.ProviderConfigSchemaSource()
}
