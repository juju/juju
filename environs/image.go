package environs

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

// FindInstanceSpec returns an InstanceSpec satisfying the supplied InstanceConstraint.
func FindInstanceSpec(r *bufio.Reader, ic *InstanceConstraint, availableTypes []InstanceType) (*InstanceSpec, error) {
	var possibleImages []Image
	var err error
	if r != nil {
		possibleImages, err = getImages(r, ic)
		if err == nil {
			for _, itype := range availableTypes {
				for _, image := range possibleImages {
					if image.match(itype) {
						return &InstanceSpec{itype.Id, itype.Name, image}, nil
					}
				}
			}
		}
	}
	// if no matching image is found for whatever reason, use the default if one is specified.
	if ic.DefaultImageId != "" && len(availableTypes) > 0 {
		spec := &InstanceSpec{
			availableTypes[0].Id, availableTypes[0].Name,
			Image{ic.DefaultImageId, ic.Arches[0], false},
		}
		return spec, nil
	}

	if len(possibleImages) == 0 || len(availableTypes) == 0 {
		return nil, fmt.Errorf(`no %q images in %s with arches %s, and no default specified`,
			ic.Series, ic.Region, ic.Arches)
	}

	names := make([]string, len(availableTypes))
	for i, itype := range availableTypes {
		names[i] = itype.Name
	}
	return nil, fmt.Errorf("no %q images in %s matching instance types %v", ic.Series, ic.Region, names)
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
