// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/juju/errors"
	"github.com/kr/pretty"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.environs.instances")

// InstanceConstraint constrains the possible instances that may be
// chosen by the environment provider.
type InstanceConstraint struct {
	Region      string
	Base        corebase.Base
	Arch        string
	Constraints constraints.Value

	// Optional filtering criteria not supported by all providers. These
	// attributes are not specified by the user as a constraint but rather
	// passed in by the provider implementation to restrict the choice of
	// available images.

	// Storage specifies a list of storage types, in order of preference.
	// eg ["ssd", "ebs"] means find images with ssd storage, but if none
	// exist, find those with ebs instead.
	Storage []string
}

// String returns a human readable form of this InstanceConstraint.
func (ic *InstanceConstraint) String() string {
	return fmt.Sprintf(
		"{region: %s, base: %s, arch: %s, constraints: %s, storage: %s}",
		ic.Region,
		ic.Base.DisplayString(),
		ic.Arch,
		ic.Constraints,
		ic.Storage,
	)
}

// InstanceSpec holds an instance type name and the chosen image info.
type InstanceSpec struct {
	InstanceType InstanceType
	Image        Image
	// order is used to sort InstanceSpec based on the input InstanceTypes.
	order int
}

// FindInstanceSpec returns an InstanceSpec satisfying the supplied InstanceConstraint.
// possibleImages contains a list of images matching the InstanceConstraint.
// allInstanceTypes provides information on every known available instance type (name, memory, cpu cores etc) on
// which instances can be run. The InstanceConstraint is used to filter allInstanceTypes and then a suitable image
// compatible with the matching instance types is returned.
func FindInstanceSpec(possibleImages []Image, ic *InstanceConstraint, allInstanceTypes []InstanceType) (*InstanceSpec, error) {
	logger.Debugf(context.TODO(), "instance constraints %+v", ic)
	if len(possibleImages) == 0 {
		return nil, errors.Errorf("no metadata for %q images in %s with arch %s",
			ic.Base.DisplayString(), ic.Region, ic.Arch)
	}

	logger.Debugf(context.TODO(), "matching constraints %v against possible image metadata %s", ic, pretty.Sprint(possibleImages))
	// If no constraints arch is specified, we need to ensure instances are filtered
	// on the arch of the agent binary.
	cons := ic.Constraints
	if !cons.HasArch() && ic.Arch != "" {
		cons.Arch = &ic.Arch
	}
	matchingTypes, err := MatchingInstanceTypes(allInstanceTypes, ic.Region, cons)
	if err != nil {
		return nil, err
	}
	if len(matchingTypes) == 0 {
		return nil, errors.Errorf("no instance types found matching constraint: %s", ic)
	}

	// We check for exact matches (all attributes matching), and also for
	// partial matches (instance type specifies attribute, but image does
	// not). Exact matches always take precedence.
	var exactSpecs, partialSpecs []*InstanceSpec
	for _, itype := range matchingTypes {
		for _, image := range possibleImages {
			specs := &partialSpecs
			if match := image.match(itype); match == exactMatch {
				specs = &exactSpecs
			} else if match == nonMatch {
				continue
			}
			*specs = append(*specs, &InstanceSpec{
				InstanceType: itype,
				Image:        image,
				order:        len(*specs),
			})
		}
	}

	specs := exactSpecs
	if len(specs) == 0 {
		specs = partialSpecs
	}
	if len(specs) > 0 {
		var specsWithoutSev []*InstanceSpec
		var specsWithSev []*InstanceSpec
		for _, spec := range specs {
			if spec.InstanceType.IsSev {
				specsWithSev = append(specsWithSev, spec)
			} else {
				specsWithoutSev = append(specsWithoutSev, spec)
			}
		}
		sort.Sort(byArch(specsWithoutSev))
		sort.Sort(byArch(specsWithSev))
		specs = slices.Concat(specsWithoutSev, specsWithSev)
		logger.Infof(context.TODO(), "find instance - using %v image of type %v with id: %v", specs[0].Image.Arch, specs[0].InstanceType.Name, specs[0].Image.Id)
		return specs[0], nil
	}

	names := make([]string, len(matchingTypes))
	for i, itype := range matchingTypes {
		names[i] = itype.Name
	}
	return nil, errors.Errorf("no %q images in %s matching instance types %v", ic.Base.DisplayString(), ic.Region, names)
}

// byArch sorts InstanceSpecs first by descending word-size, then
// alphabetically by name, and choose the first spec in the sequence.
type byArch []*InstanceSpec

func (a byArch) Len() int {
	return len(a)
}

func (a byArch) Less(i, j int) bool {
	iArchName := a[i].Image.Arch
	jArchName := a[j].Image.Arch
	// Alphabetically by arch name.
	switch {
	case iArchName < jArchName:
		return true
	case iArchName > jArchName:
		return false
	}
	// If word-size and name the same, keep stable.
	return a[i].order < a[j].order
}

func (a byArch) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Image holds the attributes that vary amongst relevant images for
// a given series in a given region.
type Image struct {
	Id   string
	Arch string
	// The type of virtualisation supported by this image.
	VirtType string
}

type imageMatch int

const (
	nonMatch imageMatch = iota
	exactMatch
	partialMatch
)

// match returns true if the image can run on the supplied instance type.
func (image Image) match(itype InstanceType) imageMatch {
	if itype.Arch != image.Arch {
		return nonMatch
	}
	if itype.VirtType == nil || image.VirtType == *itype.VirtType {
		return exactMatch
	}
	if image.VirtType == "" {
		// Image doesn't specify virtualisation type. We allow it
		// to match, but prefer exact matches.
		return partialMatch
	}
	return nonMatch
}

// ImageMetadataToImages converts an array of ImageMetadata pointers (as
// returned by imagemetadata.Fetch) to an array of Image objects (as required
// by instances.FindInstanceSpec).
func ImageMetadataToImages(inputs []*imagemetadata.ImageMetadata) []Image {
	result := make([]Image, len(inputs))
	for index, input := range inputs {
		result[index] = Image{
			Id:       input.Id,
			VirtType: input.VirtType,
			Arch:     input.Arch,
		}
	}
	return result
}
