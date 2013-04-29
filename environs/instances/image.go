package instances

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"net/http"
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
func FindInstanceSpec(r io.Reader, ic *InstanceConstraint, allInstanceTypes []InstanceType) (*InstanceSpec, error) {
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

type ImageMetadata struct {
	Id         string `json:"id"`
	Storage    string `json:"root_store"`
	VType      string `json:"virt"`
	RegionName string `json:"crsn"`
}

type ImageCollection struct {
	Images      map[string]ImageMetadata `json:"items"`
	PublicName  string                   `json:"pubname"`
	PublicLabel string                   `json:"publabel"`
	Tag         string                   `json:"label"`
}

type ImagesByVersion map[string]ImageCollection

type ImageMetadataCatalog struct {
	Release string          `json:"release"`
	Version string          `json:"version"`
	Arch    string          `json:"arch"`
	Images  ImagesByVersion `json:"versions"`
}

type CloudImageMetadata struct {
	Products map[string]ImageMetadataCatalog `json:"products"`
}

var baseImagesUrl = "http://cloud-images.ubuntu.com/eightprotons"

func GetImages(providerLabel string, e *environs.Environ, cfg *config.Config, ic *InstanceConstraint) ([]Image, error) {
	if !strings.HasSuffix(baseImagesUrl, "/") {
		baseImagesUrl += "/"
	}
	imageFilePath := fmt.Sprintf("streams/v1/com.ubuntu.cloud:released:%s.js", providerLabel)
	imageFileUrl := baseImagesUrl + imageFilePath
	resp, err := http.Get(imageFileUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("%s", resp.Status)
		return nil, err
	}
	return getImages(resp.Body, ic)
}

func findMatchingImage(images map[string]ImageMetadata, ic *InstanceConstraint) *Image {
	for _, im := range images {
		if ic.Storage != nil && im.Storage != *ic.Storage {
			continue
		}
		return &Image{
			Id: im.Id,
			VType: im.VType,
		}
	}
	return nil
}

// getImages returns the latest released ubuntu server images for the
// supplied series in the supplied region.
// r is a reader for an JSON encoded image metadata file.
func getImages(r io.Reader, ic *InstanceConstraint) ([]Image, error) {
	var images []Image
	respData, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read image metadata file: %s", err.Error())
	}

	fmt.Println(string(respData))

	var metadata CloudImageMetadata
	if len(respData) > 0 {
		err = json.Unmarshal(respData, &metadata)
		if err != nil {
			return nil, fmt.Errorf("cannot unmarshal JSON image metadata: %s", err.Error())
		}
	}
	for _, metadataCatalog := range metadata.Products {
		if metadataCatalog.Release != ic.Series {
			continue
		}
		if len(filterArches([]string{metadataCatalog.Arch}, ic.Arches)) == 0 {
			continue
		}

		// Sort the image metadata by version and look for a matching image.
		// Because of the sorting we will always return the most recent image metadata.
		bv := byVersion{}
		bv.versions = make([]string, len(metadataCatalog.Images))
		bv.imageCollections = make([]ImageCollection, len(metadataCatalog.Images))
		i := 0
		for k, v := range metadataCatalog.Images {
			bv.versions[i] = k
			bv.imageCollections[i] = v
			i++
		}
		sort.Sort(bv)
		for _, imageCollection := range bv.imageCollections {
			if image := findMatchingImage(imageCollection.Images, ic); image != nil {
				image.Arch = metadataCatalog.Arch
				images = append(images, *image)
			}
		}
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no %q images in %s with arches %v", ic.Series, ic.Region, ic.Arches)
	}
	sort.Sort(byArch(images))
	return images, nil
}

// byArch is used to sort a slice of images by architecture preference, such
// that amd64 images come earlier than i386 ones.
type byArch []Image

func (ba byArch) Len() int      { return len(ba) }
func (ba byArch) Swap(i, j int) { ba[i], ba[j] = ba[j], ba[i] }
func (ba byArch) Less(i, j int) bool {
	return ba[i].Arch == "amd64" && ba[j].Arch != "amd64"
}

// byVersion is used to sort a slice of image collections as a side effect of
// sorting a matching slice of versions in YYYYMMDD.
type byVersion struct {
	versions         []string
	imageCollections []ImageCollection
}

func (bv byVersion) Len() int { return len(bv.imageCollections) }
func (bv byVersion) Swap(i, j int) {
	bv.versions[i], bv.versions[j] = bv.versions[j], bv.versions[i]
	bv.imageCollections[i], bv.imageCollections[j] = bv.imageCollections[j], bv.imageCollections[i]
}
func (bv byVersion) Less(i, j int) bool {
	return bv.versions[i] < bv.versions[j]
}
