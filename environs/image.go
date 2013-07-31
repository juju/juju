// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
)

// ImageMetadataToImages converts an array of ImageMetadata pointers (as
// returned by imagemetadata.Fetch) to an array of Image objects (as required
// by instances.FindInstanceSpec).
func ImageMetadataToImages(inputs []*imagemetadata.ImageMetadata) []instances.Image {
	result := make([]instances.Image, len(inputs))
	for index, input := range inputs {
		result[index] = instances.Image{
			Id:    input.Id,
			VType: input.VType,
			Arch:  input.Arch,
		}
	}
	return result
}
