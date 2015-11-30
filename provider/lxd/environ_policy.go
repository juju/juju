// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
)

var supportedContainerTypes = []string{
	"lxd",
}

type policyProvider interface {
	// SupportedArchitectures returns the list of image architectures
	// supported by this environment.
	SupportedArchitectures() ([]string, error)
}

type lxdPolicyProvider struct{}

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (pp *lxdPolicyProvider) SupportedArchitectures() ([]string, error) {
	// TODO(ericsnow) Use common.SupportedArchitectures?
	localArch := arch.HostArch()
	return []string{localArch}, nil
}

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if _, err := env.parsePlacement(placement); err != nil {
		return errors.Trace(err)
	}

	if cons.HasInstanceType() {
		return errors.Errorf("LXD does not support instance types (got %q)", *cons.InstanceType)
	}

	return nil
}

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (env *environ) SupportedArchitectures() ([]string, error) {
	// TODO(ericsnow) The supported arch depends on the targetted
	// remote. Thus we may need to support the remote as a constraint.
	arches, err := env.raw.SupportedArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return arches, nil
}

var unsupportedConstraints = []string{
	constraints.CpuCores,
	constraints.CpuPower,
	//TODO(ericsnow) Add constraints.Mem as unsupported?
	constraints.InstanceType,
	constraints.Tags,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	// Register conflicts.

	// We don't have any conflicts to register.

	// Register unsupported constraints.

	validator.RegisterUnsupported(unsupportedConstraints)

	// Register the constraints vocab.

	// TODO(ericsnow) This depends on the targetted remote host.
	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)

	// TODO(ericsnow) Get this working...
	//validator.RegisterVocabulary(constraints.Container, supportedContainerTypes)

	return validator, nil
}

// environ provides SupportsUnitPlacement (a method of the
// state.EnvironCapatability interface) by embedding
// common.SupportsUnitPlacementPolicy.

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netID network.Id) (bool, error) {
	return false, nil
}
