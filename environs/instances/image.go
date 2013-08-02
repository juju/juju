// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
)

// InstanceConstraint constrains the possible instances that may be
// chosen by the environment provider.
type InstanceConstraint struct {
	Region      string
	Series      string
	Arches      []string
	Constraints constraints.Value
	// Optional filtering criteria not supported by all providers. These attributes are not specified
	// by the user as a constraint but rather passed in by the provider implementation to restrict the
	// choice of available images.
	Storage *string
}

// String returns a human readable form of this InstanceConstaint.
func (ic *InstanceConstraint) String() string {
	storage := "none"
	if ic.Storage != nil {
		storage = *ic.Storage
	}
	return fmt.Sprintf(
		"{region: %s, series: %s, arches: %s, constraints: %s, storage: %s}",
		ic.Region,
		ic.Series,
		ic.Arches,
		ic.Constraints,
		storage,
	)
}

// InstanceSpec holds an instance type name and the chosen image info.
type InstanceSpec struct {
	InstanceType InstanceType
	Image        Image
}

// FindInstanceSpec returns an InstanceSpec satisfying the supplied InstanceConstraint.
// possibleImages contains a list of images matching the InstanceConstraint.
// allInstanceTypes provides information on every known available instance type (name, memory, cpu cores etc) on
// which instances can be run. The InstanceConstraint is used to filter allInstanceTypes and then a suitable image
// compatible with the matching instance types is returned.
func FindInstanceSpec(possibleImages []Image, ic *InstanceConstraint, allInstanceTypes []InstanceType) (*InstanceSpec, error) {
	matchingTypes, err := getMatchingInstanceTypes(ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}

	for _, itype := range matchingTypes {
		for _, image := range possibleImages {
			if image.match(itype) {
				return &InstanceSpec{itype, image}, nil
			}
		}
	}

	if len(possibleImages) == 0 || len(matchingTypes) == 0 {
		return nil, fmt.Errorf("no %q images in %s with arches %s",
			ic.Series, ic.Region, ic.Arches)
	}

	names := make([]string, len(matchingTypes))
	for i, itype := range matchingTypes {
		names[i] = itype.Name
	}
	return nil, fmt.Errorf("no %q images in %s matching instance types %v", ic.Series, ic.Region, names)
}

// Image holds the attributes that vary amongst relevant images for
// a given series in a given region.
type Image struct {
	Id   string
	Arch string
	// The type of virtualisation supported by this image.
	VType string
}

// match returns true if the image can run on the supplied instance type.
func (image Image) match(itype InstanceType) bool {
	// The virtualisation type is optional.
	if itype.VType != nil && image.VType != *itype.VType {
		return false
	}
	for _, arch := range itype.Arches {
		if arch == image.Arch {
			return true
		}
	}
	return false
}

// ImageMetadataToImages converts an array of ImageMetadata pointers (as
// returned by imagemetadata.Fetch) to an array of Image objects (as required
// by instances.FindInstanceSpec).
func ImageMetadataToImages(inputs []*imagemetadata.ImageMetadata) []Image {
	result := make([]Image, len(inputs))
	for index, input := range inputs {
		result[index] = Image{
			Id:    input.Id,
			VType: input.VType,
			Arch:  input.Arch,
		}
	}
	return result
}
