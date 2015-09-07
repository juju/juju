// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/juju/loggo"

	"github.com/juju/errors"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/juju/arch"
)

var logger = loggo.GetLogger("juju.environs.instances")

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

	// Storage specifies a list of storage types, in order of preference.
	// eg ["ssd", "ebs"] means find images with ssd storage, but if none exist,
	// find those with ebs instead.
	Storage []string
}

// String returns a human readable form of this InstanceConstraint.
func (ic *InstanceConstraint) String() string {
	return fmt.Sprintf(
		"{region: %s, series: %s, arches: %s, constraints: %s, storage: %s}",
		ic.Region,
		ic.Series,
		ic.Arches,
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

type ImagePredicateFn func(Image) bool

func (p ImagePredicateFn) And(preds ...ImagePredicateFn) ImagePredicateFn {
	return func(i Image) bool {
		for _, p := range append([]ImagePredicateFn{p}, preds...) {
			if p == nil {
				continue
			} else if p(i) == false {
				return false
			}
		}
		return true
	}
}

func (p ImagePredicateFn) Or(preds ...ImagePredicateFn) ImagePredicateFn {
	return func(i Image) bool {
		for _, p := range append([]ImagePredicateFn{p}, preds...) {
			if p == nil {
				continue
			} else if p(i) {
				return true
			}
		}
		return false
	}
}

func ImgMatchesInstanceType(instanceType InstanceType) ImagePredicateFn {
	return func(image Image) bool {
		return image.match(instanceType)
	}
}

func ImgHasArch(arch *string) ImagePredicateFn {
	return func(i Image) bool {
		return arch == nil || *arch == i.Arch
	}
}

func ImgHasVirtType(virtType *string) ImagePredicateFn {
	return func(i Image) bool {
		return virtType == nil || *virtType == i.VirtType
	}
}

func ImgMatchesConstraint(constraint constraints.Value) ImagePredicateFn {
	return ImagePredicateFn.And(
		ImgHasArch(constraint.Arch),
	)
}

func FilterImages(possibleImages []Image, matches ImagePredicateFn) []Image {
	goodInstances := make([]Image, 0, len(possibleImages))
	for _, image := range possibleImages {
		if matches(image) == false {
			continue
		}
		goodInstances = append(goodInstances, image)
	}

	return goodInstances
}

// FindInstanceSpec returns an InstanceSpec satisfying the supplied
// InstanceConstraint. possibleImages contains a list of images
// matching the InstanceConstraint. allInstanceTypes provides
// information on every known available instance type (name, memory,
// cpu cores etc) on which instances can be run. The
// InstanceConstraint is used to filter allInstanceTypes and then a
// suitable image compatible with the matching instance types is
// returned.
func FindInstanceSpec(
	possibleImages []Image,
	ic *InstanceConstraint,
	instanceTypes []InstanceType,
) (*InstanceSpec, error) {

	for _, t := range instanceTypes {
		fmt.Fprintf(&DebugBuffer, "(1)instanceType: %+v\n", t)
		if t.VirtType != nil {
			fmt.Fprintf(&DebugBuffer, "\t(1)virtType: %s\n", *t.VirtType)
		}
		if t.CpuPower != nil {
			fmt.Fprintf(&DebugBuffer, "\t(1)cpuPower: %d\n", *t.CpuPower)
		}
	}

	matches := MatchesConstraintsOrMinMem(ic.Constraints, minMemoryHeuristic)

	instanceTypes = FilterInstanceTypes(instanceTypes, matches)
	if len(instanceTypes) <= 0 {
		return nil, fmt.Errorf("no instance types in %s matching constraints %q", ic.Region, ic.Constraints)
	}
	sort.Sort(byCost(instanceTypes))

	var imagesFilter ImagePredicateFn
	for _, instanceType := range instanceTypes {
		imagesFilter = ImagePredicateFn.Or(
			imagesFilter,
			ImgMatchesInstanceType(instanceType),
		)
	}
	imagesFilter = ImagePredicateFn.And(imagesFilter, ImgMatchesConstraint(ic.Constraints))

	possibleImages = FilterImages(possibleImages, imagesFilter)

	bestImages := findAlphaNumericallyFirstArchName(
		findLargestWordSize(
			possibleImages,
			func(a string) int { return arch.Info[a].WordSize },
		),
	)
	if len(bestImages) <= 0 {
		fmt.Fprintf(&DebugBuffer, "(A)instanceTypes: %v\n", instanceTypes)
		fmt.Fprintf(&DebugBuffer, "(A)possibleImages: %v\n", possibleImages)
		return nil, generateImagesNotFoundError(ic, instanceTypes)
	}
	bestImage := bestImages[0]

	for _, instanceType := range instanceTypes {
		if bestImage.match(instanceType) {
			for _, t := range instanceTypes {
				fmt.Fprintf(&DebugBuffer, "(C)instanceType: %+v\n", t)
				if t.VirtType != nil {
					fmt.Fprintf(&DebugBuffer, "\t(C)virtType: %s\n", *t.VirtType)
				}
				if t.CpuPower != nil {
					fmt.Fprintf(&DebugBuffer, "\t(C)cpuPower: %d\n", *t.CpuPower)
				}
			}
			fmt.Fprintf(&DebugBuffer, "(C)possibleImages: %+v\n", possibleImages)
			return &InstanceSpec{
				InstanceType: instanceType,
				Image:        bestImage,
			}, nil
		}
	}

	fmt.Printf("(B) possibleImages: %v", possibleImages)
	return nil, generateImagesNotFoundError(ic, instanceTypes)
}

var DebugBuffer bytes.Buffer

func generateImagesNotFoundError(constraint *InstanceConstraint, instanceTypes []InstanceType) error {
	names := make([]string, 0, len(instanceTypes))
	for _, itype := range instanceTypes {
		names = append(names, itype.Name)
	}
	return fmt.Errorf("no %q images in %s matching instance types %v",
		constraint.Series,
		constraint.Region,
		names,
	)
	return errors.NotFoundf(
		"%q images in %s matching instance types %v",
		constraint.Series,
		constraint.Region,
		instanceTypes,
	)
}

func findAlphaNumericallyFirstArchName(images []Image) []Image {
	if len(images) <= 0 {
		return nil
	}
	bestImage := []Image{images[0]}
	for _, image := range images[1:] {
		switch archName := image.Arch; {
		case archName < bestImage[0].Arch:
			bestImage = []Image{image}
		}
	}

	return bestImage
}

func findLargestWordSize(images []Image, wordSize func(string) int) []Image {
	if len(images) <= 0 {
		return nil
	}

	bestImage := []Image{images[0]}
	bestWordSize := wordSize(images[0].Arch)
	for _, image := range images[1:] {
		switch ws := wordSize(image.Arch); {
		case ws > bestWordSize:
			bestWordSize = ws
			bestImage = []Image{image}
		case ws == bestWordSize:
			bestImage = append(bestImage, image)
		}
	}
	return bestImage
}

// Image holds the attributes that vary amongst relevant images for
// a given series in a given region.
type Image struct {
	Id   string
	Arch string
	// The type of virtualisation supported by this image.
	VirtType string
}

// match returns true if the image can run on the supplied instance type.
func (image Image) match(itype InstanceType) bool {
	// The virtualisation type is optional.
	if itype.VirtType != nil && image.VirtType != *itype.VirtType {
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
			Id:       input.Id,
			VirtType: input.VirtType,
			Arch:     input.Arch,
		}
	}
	return result
}
