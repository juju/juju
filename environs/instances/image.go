package instances

import (
	"bufio"
	"fmt"
	"io"
	"launchpad.net/juju-core/constraints"
	"sort"
	"strings"
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
	// Optional constraints not supported by all providers.
	Storage *string
	Cluster *string
}

// InstanceSpec holds an instance type name and the chosen image info.
type InstanceSpec struct {
	InstanceTypeId   string
	InstanceTypeName string
	Image            Image
}

// minMemoryForMongoDB is the assumed minimum amount of memory (in MB) we require in order to run MongoDB (1GB)
const minMemoryForMongoDB = 1024

// FindInstanceSpec returns an InstanceSpec satisfying the supplied InstanceConstraint.
func FindInstanceSpec(r *bufio.Reader, ic *InstanceConstraint, allInstanceTypes []InstanceType, regionCosts RegionCosts) (*InstanceSpec, error) {
	matchingTypes, err := getMatchingInstanceTypes(ic, allInstanceTypes, regionCosts)
	if err != nil {
		// There are no instance types matching the supplied constraints. If the user has specifically
		// asked for a nominated default instance type to be used as a fallback and that is invalid, we
		// report the error. Otherwise we continue to look for an instance type that we can use as a last resort.
		if len(allInstanceTypes) == 0 || ic.DefaultInstanceType != "" {
			return nil, err
		}
		// No matching instance types were found, so the fallback is to:
		// 1. Sort by memory and find the cheapest matching both the required architecture
		//    and our own heuristic: minimum amount of memory required to run MongoDB, or
		// 2. Sort by cost in reverse order and return the most expensive one, which will hopefully work,
		//    albeit not the best match

		archCons := &InstanceConstraint{Arches: ic.Arches}
		fallbackTypes, fberr := getMatchingInstanceTypes(archCons, allInstanceTypes, regionCosts)
		// If there's an error getting the fallback instance, return the original error.
		if fberr != nil {
			return nil, err
		}
		typeByMemory := byMemory(fallbackTypes)
		sort.Sort(typeByMemory)
		// 1. check for smallest instance type that can run mongodb
		for _, itype := range typeByMemory {
			if itype.Mem >= minMemoryForMongoDB {
				matchingTypes = []InstanceType{itype}
				break
			}
		}
		if len(matchingTypes) == 0 {
			// 2. just get the one with the largest memory
			matchingTypes = []InstanceType{typeByMemory[len(typeByMemory)-1]}
		}
	}

	var possibleImages []Image
	if r != nil {
		possibleImages, err = getImages(r, ic)
		if err == nil {
			for _, itype := range matchingTypes {
				for _, image := range possibleImages {
					if image.match(itype) {
						return &InstanceSpec{itype.Id, itype.Name, image}, nil
					}
				}
			}
		}
	}
	// if no matching image is found for whatever reason, use the default if one is specified.
	if ic.DefaultImageId != "" && len(matchingTypes) > 0 {
		spec := &InstanceSpec{
			matchingTypes[0].Id, matchingTypes[0].Name,
			Image{ic.DefaultImageId, ic.Arches[0], false},
		}
		return spec, nil
	}

	if len(possibleImages) == 0 || len(matchingTypes) == 0 {
		return nil, fmt.Errorf(`no %q images in %s with arches %s, and no default specified`,
			ic.Series, ic.Region, ic.Arches)
	}

	names := make([]string, len(matchingTypes))
	for i, itype := range matchingTypes {
		names[i] = itype.Name
	}
	return nil, fmt.Errorf("no %q images in %s matching instance types %v", ic.Series, ic.Region, names)
}

type byMemory []InstanceType

func (s byMemory) Len() int {
	return len(s)
}

func (s byMemory) Less(i, j int) bool {
	return s[i].Mem < s[j].Mem
}

func (s byMemory) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Columns in the file returned from the images server.
const (
	colSeries = iota
	colServer
	colDaily
	colDate
	colStorage
	colArch
	colRegion
	colImageId
	_
	_
	colVtype
	colMax
	// + more that we don't care about.
)

// Image holds the attributes that vary amongst relevant images for
// a given series in a given region.
type Image struct {
	Id   string
	Arch string
	// Clustered is true when the image is built for an cluster instance type.
	Clustered bool
}

// match returns true if the image can run on the supplied instance type.
func (image Image) match(itype InstanceType) bool {
	if image.Clustered != itype.Clustered {
		return false
	}
	for _, arch := range itype.Arches {
		if arch == image.Arch {
			return true
		}
	}
	return false
}

// getImages returns the latest released ubuntu server images for the
// supplied series in the supplied region.
func getImages(r *bufio.Reader, ic *InstanceConstraint) ([]Image, error) {
	var images []Image
	for {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			if len(images) == 0 {
				return nil, fmt.Errorf("no %q images in %s with arches %v", ic.Series, ic.Region, ic.Arches)
			}
			sort.Sort(byArch(images))
			return images, nil
		} else if err != nil {
			return nil, err
		}
		f := strings.Split(string(line), "\t")
		if len(f) < colMax {
			continue
		}
		if f[colRegion] != ic.Region {
			continue
		}
		if ic.Storage != nil && f[colStorage] != *ic.Storage {
			continue
		}
		if len(filterArches([]string{f[colArch]}, ic.Arches)) != 0 {
			var clustered bool
			if ic.Cluster != nil {
				clustered = f[colVtype] == *ic.Cluster
			}
			images = append(images, Image{
				Id:        f[colImageId],
				Arch:      f[colArch],
				Clustered: clustered,
			})
		}
	}
	panic("unreachable")
}

// byArch is used to sort a slice of images by architecture preference, such
// that amd64 images come earlier than i386 ones.
type byArch []Image

func (ba byArch) Len() int      { return len(ba) }
func (ba byArch) Swap(i, j int) { ba[i], ba[j] = ba[j], ba[i] }
func (ba byArch) Less(i, j int) bool {
	return ba[i].Arch == "amd64" && ba[j].Arch != "amd64"
}
