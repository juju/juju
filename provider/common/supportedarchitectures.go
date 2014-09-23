// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
)

// SupportedArchitectures returns all the image architectures for env matching the constraints.
func SupportedArchitectures(env environs.Environ, imageConstraint *imagemetadata.ImageConstraint) ([]string, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}
	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, false)
	if err != nil {
		return nil, err
	}
	var arches = set.NewStrings()
	for _, im := range matchingImages {
		arches.Add(im.Arch)
	}
	return arches.Values(), nil
}
