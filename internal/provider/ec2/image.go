// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"

	"github.com/kr/pretty"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

// filterImages returns only that subset of the input (in the same order) that
// this provider finds suitable.
func filterImages(ctx context.Context, images []*imagemetadata.ImageMetadata, ic *instances.InstanceConstraint) []*imagemetadata.ImageMetadata {
	// Gather the images for each available storage type.
	imagesByStorage := make(map[string][]*imagemetadata.ImageMetadata)
	for _, image := range images {
		imagesByStorage[image.Storage] = append(imagesByStorage[image.Storage], image)
	}
	logger.Debugf(ctx, "images by storage type %+v", imagesByStorage)
	// If a storage directive has been specified, use that or else default to ssd.
	storageTypes := []string{ssdStorage}
	if ic != nil && len(ic.Storage) > 0 {
		storageTypes = ic.Storage
	}
	logger.Debugf(ctx, "filtering storage types %+v", storageTypes)
	// Return the first set of images for which we have a storage type match.
	for _, storageType := range storageTypes {
		if len(imagesByStorage[storageType]) > 0 {
			return imagesByStorage[storageType]
		}
	}
	// If the user specifies an image ID during bootstrap, then it will not
	// have a storage type.
	return imagesByStorage[""]
}

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(
	ctx context.Context,
	controller bool,
	allImageMetadata []*imagemetadata.ImageMetadata,
	instanceTypes []instances.InstanceType,
	ic *instances.InstanceConstraint,
) (*instances.InstanceSpec, error) {
	logger.Debugf(ctx, "received %d image(s)", len(allImageMetadata))
	if !controller {
		ic.Constraints = withDefaultNonControllerConstraints(ic.Constraints)
	}
	suitableImages := filterImages(ctx, allImageMetadata, ic)
	logger.Debugf(ctx, "found %d suitable image(s) from %d available: %s",
		len(suitableImages),
		len(allImageMetadata),
		pretty.Sprint(suitableImages),
	)
	images := instances.ImageMetadataToImages(suitableImages)
	return instances.FindInstanceSpec(images, ic, instanceTypes)
}

// withDefaultNonControllerConstraints returns the given constraints,
// updated to choose a default instance type appropriate for a
// non-controller machine. We use this only if the user does not
// specify an instance-type, or cpu-power.
//
// At the time of writing, this will choose the cheapest non-burstable
// instance available in the account/region. At the time of writing, that
// is, for example:
//   - m3.medium (for EC2-Classic)
//   - c4.large (e.g. in ap-south-1)
func withDefaultNonControllerConstraints(cons constraints.Value) constraints.Value {
	if !cons.HasInstanceType() && !cons.HasCpuPower() {
		cons.CpuPower = instances.CpuPower(100)
	}
	return cons
}
