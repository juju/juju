package policy

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/policy"
)

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
	// Prechecker takes a *config.Config and returns a Prechecker or an error.
	Prechecker(*config.Config) (policy.Prechecker, error)

	// ConfigValidator takes a provider type name and returns a ConfigValidator
	// or an error.
	ConfigValidator(providerType string) (policy.ConfigValidator, error)

	// EnvironCapability takes a *config.Config and returns an EnvironCapability
	// or an error.
	EnvironCapability(*config.Config) (policy.EnvironCapability, error)

	// ConstraintsValidator takes a *config.Config and returns a
	// constraints.Validator or an error.
	ConstraintsValidator(*config.Config) (constraints.Validator, error)

	// InstanceDistributor takes a *config.Config and returns an
	// InstanceDistributor or an error.
	InstanceDistributor(*config.Config) (policy.InstanceDistributor, error)
}
