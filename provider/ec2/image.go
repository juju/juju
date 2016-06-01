// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

// defaultCpuPower is larger the smallest instance's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing the smallest instance from being chosen unless
// the user has clearly indicated that they are willing to accept poor performance.
const defaultCpuPower = 100

// filterImages returns only that subset of the input (in the same order) that
// this provider finds suitable.
func filterImages(images []*imagemetadata.ImageMetadata, ic *instances.InstanceConstraint) []*imagemetadata.ImageMetadata {
	// Gather the images for each available storage type.
	imagesByStorage := make(map[string][]*imagemetadata.ImageMetadata)
	for _, image := range images {
		imagesByStorage[image.Storage] = append(imagesByStorage[image.Storage], image)
	}
	logger.Debugf("images by storage type %+v", imagesByStorage)
	// If a storage constraint has been specified, use that or else default to ssd.
	storageTypes := []string{ssdStorage}
	if ic != nil && len(ic.Storage) > 0 {
		storageTypes = ic.Storage
	}
	logger.Debugf("filtering storage types %+v", storageTypes)
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
	allImageMetadata []*imagemetadata.ImageMetadata,
	ic *instances.InstanceConstraint,
) (*instances.InstanceSpec, error) {
	logger.Debugf("received %d image(s)", len(allImageMetadata))
	// If the instance type is set, don't also set a default CPU power
	// as this is implied.
	cons := ic.Constraints
	if cons.CpuPower == nil && (cons.InstanceType == nil || *cons.InstanceType == "") {
		ic.Constraints.CpuPower = instances.CpuPower(defaultCpuPower)
	}
	suitableImages := filterImages(allImageMetadata, ic)
	logger.Debugf("found %d suitable image(s)", len(suitableImages))
	images := instances.ImageMetadataToImages(suitableImages)

	// Make a copy of the known EC2 instance types, filling in the cost for the specified region.
	regionCosts := allRegionCosts[ic.Region]
	if len(regionCosts) == 0 && len(allRegionCosts) > 0 {
		return nil, fmt.Errorf("no instance types found in %s", ic.Region)
	}

	var itypesWithCosts []instances.InstanceType
	for _, itype := range allInstanceTypes {
		cost, ok := regionCosts[itype.Name]
		if !ok {
			continue
		}
		itWithCost := itype
		itWithCost.Cost = cost
		itypesWithCosts = append(itypesWithCosts, itWithCost)
	}
	return instances.FindInstanceSpec(images, ic, itypesWithCosts)
}
