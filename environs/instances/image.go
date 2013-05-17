// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
)

// InstanceConstraint constrains the possible instances that may be
// chosen by the environment provider.
type InstanceConstraint struct {
	Region              string
	Series              string
	Arches              []string
	Constraints         constraints.Value
	DefaultInstanceType string // the default instance type to use if none matches the constraints
	DefaultImageId      string // the default image to use if none matches the constraints
	// Optional filtering criteria not supported by all providers. These attributes are not specified
	// by the user as a constraint but rather passed in by the provider implementation to restrict the
	// choice of available images.
	Storage *string
}

// InstanceSpec holds an instance type name and the chosen image info.
type InstanceSpec struct {
	InstanceTypeId   string
	InstanceTypeName string
	Image            Image
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
		typeMatch := false
		for _, image := range possibleImages {
			if image.match(itype) {
				typeMatch = true
				if ic.DefaultImageId == "" || ic.DefaultImageId == image.Id {
					return &InstanceSpec{itype.Id, itype.Name, image}, nil
				}
			}
		}
		if typeMatch && ic.DefaultImageId != "" {
			return nil, fmt.Errorf("invalid default image id %q", ic.DefaultImageId)
		}
	}
	// if no matching image is found for whatever reason, use the default if one is specified.
	if ic.DefaultImageId != "" && len(matchingTypes) > 0 {
		spec := &InstanceSpec{
			InstanceTypeId:   matchingTypes[0].Id,
			InstanceTypeName: matchingTypes[0].Name,
			Image:            Image{Id: ic.DefaultImageId, Arch: ic.Arches[0]},
		}
		return spec, nil
	}

	if len(possibleImages) == 0 || len(matchingTypes) == 0 {
		return nil, fmt.Errorf("no %q images in %s with arches %s, and no default specified",
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
