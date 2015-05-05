// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/network"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		if _, err := env.parsePlacement(placement); err != nil {
			return err
		}
	}
	return nil
}

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (env *environ) SupportedArchitectures() ([]string, error) {
	env.archLock.Lock()
	defer env.archLock.Unlock()

	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}

	archList, err := env.lookupArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.supportedArchitectures = archList
	return archList, nil
}

func (env *environ) lookupArchitectures() ([]string, error) {
	// Create a filter to get all images for the
	// correct stream.
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Stream: env.Config().ImageStream(),
	})
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	matchingImages, err := imageMetadataFetch(sources, imageConstraint)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var arches = set.NewStrings()
	for _, im := range matchingImages {
		arches.Add(im.Arch)
	}

	return arches.Values(), nil
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.Networks,
}

// instanceTypeConstraints defines the fields defined on each of the
// instance types.  See instancetypes.go.
var instanceConstraints = []string{
	constraints.Arch, // Arches
	constraints.CpuCores,
	constraints.CpuPower,
	constraints.Mem,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	// unsupported

	validator.RegisterUnsupported(unsupportedConstraints)

	// vocab

	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)

	return validator, nil
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
func (env *environ) SupportAddressAllocation(netID network.Id) (bool, error) {
	return false, nil
}
