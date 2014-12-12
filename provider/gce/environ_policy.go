package gce

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return errNotImplemented
}

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errNotImplemented
}

// SupportsUnitPlacement implement via common.SupportsUnitPlacementPolicy

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netId network.Id) (bool, error) {
	return false, nil
}
