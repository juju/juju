// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
)

// SupportedArchitectures returns distinct image architectures for env
// matching given constraints.
func SupportedArchitectures(env environs.Environ, imageConstraint *imagemetadata.ImageConstraint, knownArchitectures []string) ([]string, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}
	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint)
	if err != nil {
		return nil, err
	}

	result := set.NewStrings(knownArchitectures...)
	acceptedArchitectures := set.NewStrings(imageConstraint.Arches...)
	if len(imageConstraint.Arches) != 0 {
		for _, knownOne := range knownArchitectures {
			// Only keep architectures that match given constraint.
			if !acceptedArchitectures.Contains(knownOne) {
				result.Remove(knownOne)
			}
		}
	}
	for _, im := range matchingImages {
		result.Add(im.Arch)
	}
	return result.SortedValues(), nil
}
