// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
        "launchpad.net/loggo"	
)

var logger = loggo.GetLogger("juju.environs.ec2")

// signedImageDataOnly is defined here to allow tests to override the content.
// If true, only inline PGP signed image metadata will be used.
var signedImageDataOnly = true

// defaultCpuPower is larger the smallest instance's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing the smallest instance from being chosen unless
// the user has clearly indicated that they are willing to accept poor performance.
const defaultCpuPower = 100

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func findInstanceSpec(baseURLs []string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	if ic.Constraints.CpuPower == nil {
		ic.Constraints.CpuPower = instances.CpuPower(defaultCpuPower)
	}
	ec2Region := allRegions[ic.Region]
	imageConstraint := imagemetadata.ImageConstraint{
		CloudSpec: imagemetadata.CloudSpec{ic.Region, ec2Region.EC2Endpoint},
		Series:    ic.Series,
		Arches:    ic.Arches,
	}
	matchingImages, err := imagemetadata.Fetch(
		baseURLs, imagemetadata.DefaultIndexPath, &imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, err
	}
	if len(matchingImages) == 0 {
		logger.Warningf("no matching image meta data for constraints: %v", ic)
	}
	var images []instances.Image
	for _, imageMetadata := range matchingImages {
		// For now, we only want images with "ebs" storage.
		if imageMetadata.Storage != ebsStorage {
			continue
		}
		im := *imageMetadata
		images = append(images, instances.Image{
			Id:    im.Id,
			VType: im.VType,
			Arch:  im.Arch,
		})
	}

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
