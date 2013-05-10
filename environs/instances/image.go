package instances

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
	"sort"
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

// minMemoryHeuristic is the assumed minimum amount of memory (in MB) we prefer in order to run a server (1GB)
const minMemoryHeuristic = 1024

// FindInstanceSpec returns an InstanceSpec satisfying the supplied InstanceConstraint.
// r has been set up to read from a file containing Ubuntu cloud guest images availability data. A query
// interface for EC2 images is exposed at http://cloud-images.ubuntu.com/query. Other cloud providers may
// provide similar files for their own images. e.g. the Openstack provider has been configured to look for
// cloud image availability files in the cloud's control and public storage containers.
// For more information on the image availability file format, see https://help.ubuntu.com/community/UEC/Images.
// allInstanceTypes provides information on every known available instance type (name, memory, cpu cores etc) on
// which instances can be run.
func FindInstanceSpec(possibleImages []Image, ic *InstanceConstraint, allInstanceTypes []InstanceType) (*InstanceSpec, error) {
	matchingTypes, err := getMatchingInstanceTypes(ic, allInstanceTypes)
	if err != nil {
		// There are no instance types matching the supplied constraints. If the user has specifically
		// asked for a nominated default instance type to be used as a fallback and that is invalid, we
		// report the error. Otherwise we continue to look for an instance type that we can use as a last resort.
		if len(allInstanceTypes) == 0 || ic.DefaultInstanceType != "" {
			return nil, err
		}
		// No matching instance types were found, so the fallback is to:
		// 1. Sort by memory and find the smallest matching both the required architecture
		//    and our own heuristic: minimum amount of memory required to run a realistic server, or
		// 2. Sort by memory in reverse order and return the largest one, which will hopefully work,
		//    albeit not the best match

		archCons := &InstanceConstraint{Arches: ic.Arches}
		fallbackTypes, fberr := getMatchingInstanceTypes(archCons, allInstanceTypes)
		// If there's an error getting the fallback instance, return the original error.
		if fberr != nil {
			return nil, err
		}
		sort.Sort(byMemory(fallbackTypes))
		// 1. check for smallest instance type that can realistically run a server
		for _, itype := range fallbackTypes {
			if itype.Mem >= minMemoryHeuristic {
				matchingTypes = []InstanceType{itype}
				break
			}
		}
		if len(matchingTypes) == 0 {
			// 2. just get the one with the largest memory
			matchingTypes = []InstanceType{fallbackTypes[len(fallbackTypes)-1]}
		}
	}

	sort.Sort(byArch(possibleImages))
	for _, itype := range matchingTypes {
		for _, image := range possibleImages {
			if image.match(itype) {
				return &InstanceSpec{itype.Id, itype.Name, image}, nil
			}
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

//byMemory is used to sort a slice of instance types by the amount of RAM they have.
type byMemory []InstanceType

func (s byMemory) Len() int      { return len(s) }
func (s byMemory) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byMemory) Less(i, j int) bool {
	return s[i].Mem < s[j].Mem
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

// byArch is used to sort a slice of images by architecture preference, such
// that amd64 images come earlier than i386 ones.
type byArch []Image

func (ba byArch) Len() int      { return len(ba) }
func (ba byArch) Swap(i, j int) { ba[i], ba[j] = ba[j], ba[i] }
func (ba byArch) Less(i, j int) bool {
	return ba[i].Arch == "amd64" && ba[j].Arch != "amd64"
}
