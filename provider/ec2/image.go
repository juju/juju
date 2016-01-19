// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
)

var (
	// signedImageDataOnly is defined here to allow tests to override the content.
	// If true, only inline PGP signed image metadata will be used.
	signedImageDataOnly = true

	// defaultInstanceTypeRef is the default EC2 instance we'd like to
	// use as a reference when specifying default CpuPower and Mem
	// constraints. This is only referenced if InstanceType is not
	// specified.
	defaultInstanceTypeRef = findInstanceTypeWithName("m3.medium", allInstanceTypes...)
)

// filterImages returns only that subset of the input (in the same order) that
// this provider finds suitable.
func filterImages(images []*imagemetadata.ImageMetadata, ic *instances.InstanceConstraint) []*imagemetadata.ImageMetadata {
	// Gather the images for each available storage type.
	imagesByStorage := make(map[string][]*imagemetadata.ImageMetadata)
	for _, image := range images {
		imagesByStorage[image.Storage] = append(imagesByStorage[image.Storage], image)
	}
	// If a storage constraint has been specified, use that or else default to ssd.
	storageTypes := []string{ssdStorage}
	if ic != nil && len(ic.Storage) > 0 {
		storageTypes = ic.Storage
	}
	// Return the first set of images for which we have a storage type match.
	for _, storageType := range storageTypes {
		if len(imagesByStorage[storageType]) > 0 {
			return imagesByStorage[storageType]
		}
	}
	return nil
}

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(
	sources []simplestreams.DataSource, stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {

	// If the instance type is set, don't also set a default CPU power
	// as this is implied.
	cons := ic.Constraints
	if cons.InstanceType == nil || *cons.InstanceType == "" {
		if cons.CpuPower == nil {
			cons.CpuPower = defaultInstanceTypeRef.CpuPower
		}
		if cons.Mem == nil {
			cons.Mem = &defaultInstanceTypeRef.Mem
		}
	}

	ec2Region := allRegions[ic.Region]
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{ic.Region, ec2Region.EC2Endpoint},
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    stream,
	})
	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, err
	}
	if len(matchingImages) == 0 {
		logger.Warningf("no matching image meta data for constraints: %v", ic)
	}
	suitableImages := filterImages(matchingImages, ic)
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
