// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state/cloudimagemetadata"
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
	// ConfigValidator returns a config.Validator or an error.
	ConfigValidator() (config.Validator, error)

	// ConstraintsValidator returns a constraints.Validator or an error.
	ConstraintsValidator(envcontext.ProviderCallContext) (constraints.Validator, error)

	// StorageServices returns a StoragePoolGetter, storage.ProviderRegistry or an error.
	StorageServices() (StoragePoolGetter, storage.ProviderRegistry, error)
}

func (st *State) constraintsValidator() (constraints.Validator, error) {
	// Default behaviour is to simply use a standard validator with
	// no model specific behaviour built in.
	var validator constraints.Validator
	if st.policy != nil {
		var err error
		validator, err = st.policy.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(stdcontext.Background()))
		if errors.Is(err, errors.NotImplemented) {
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
		cfg, err := model.ModelConfig(stdcontext.Background())
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

		// Also include any arches that belong to manually provisioned machines
		// See LP1946639.
		manualMachArches, err := st.GetManualMachineArches()
		if err != nil {
			return nil, errors.Annotate(err, "querying supported architectures for manual machines")
		}

		if supportedArches := manualMachArches.Union(set.NewStrings(arches...)); len(supportedArches) != 0 {
			validator.UpdateVocabulary(constraints.Arch, supportedArches.SortedValues())
		}
	}

	return validator, nil
}

// ResolveConstraints combines the given constraints with the environ constraints to get
// a constraints which will be used to create a new instance.
func (st *State) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	validator, err := st.constraintsValidator()
	if err != nil {
		return constraints.Value{}, err
	}
	modelCons, err := st.ModelConstraints()
	if err != nil {
		return constraints.Value{}, err
	}
	return validator.Merge(modelCons, cons)
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
	if errors.Is(err, errors.NotImplemented) {
		return cfg, nil
	} else if err != nil {
		return nil, err
	}
	if configValidator == nil {
		return nil, errors.New("policy returned nil configValidator without an error")
	}
	return configValidator.Validate(cfg, old)
}

// Used for tests.
type noopStoragePoolGetter struct{}

func (noopStoragePoolGetter) GetStoragePoolByName(ctx stdcontext.Context, name string) (*storage.Config, error) {
	return nil, fmt.Errorf("storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError))
}

func (st *State) storageServices() (StoragePoolGetter, storage.ProviderRegistry, error) {
	if st.policy == nil {
		return noopStoragePoolGetter{}, storage.StaticProviderRegistry{}, nil
	}
	return st.policy.StorageServices()
}
