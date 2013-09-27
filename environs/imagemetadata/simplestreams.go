// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The imagemetadata package supports locating, parsing, and filtering Ubuntu image metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package imagemetadata

import (
	"fmt"

	"launchpad.net/juju-core/environs/simplestreams"
)

func init() {
	simplestreams.RegisterStructTags(ImageMetadata{})
}

const (
	ImageIds = "image-ids"
)

// This needs to be a var so we can override it for testing.
var DefaultBaseURL = "http://cloud-images.ubuntu.com/releases"

// ImageConstraint defines criteria used to find an image metadata record.
type ImageConstraint struct {
	simplestreams.LookupParams
}

func NewImageConstraint(params simplestreams.LookupParams) *ImageConstraint {
	if len(params.Series) != 1 {
		// This can only happen as a result of a coding error.
		panic(fmt.Sprintf("image constraint requires a single series, got %v", params.Series))
	}
	return &ImageConstraint{LookupParams: params}
}

// Generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
func (ic *ImageConstraint) Ids() ([]string, error) {
	stream := ic.Stream
	if stream != "" {
		stream = "." + stream
	}
	version, err := simplestreams.SeriesVersion(ic.Series[0])
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(ic.Arches))
	for i, arch := range ic.Arches {
		ids[i] = fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, arch)
	}
	return ids, nil
}

// ImageMetadata holds information about a particular cloud image.
type ImageMetadata struct {
	Id          string `json:"id"`
	Storage     string `json:"root_store"`
	VType       string `json:"virt"`
	Arch        string `json:"arch"`
	RegionAlias string `json:"crsn"`
	RegionName  string `json:"region"`
	Endpoint    string `json:"endpoint"`
}

// Fetch returns a list of images for the specified cloud matching the constraint.
// The base URL locations are as specified - the first location which has a file is the one used.
// Signed data is preferred, but if there is no signed data available and onlySigned is false,
// then unsigned data is used.
func Fetch(sources []simplestreams.DataSource, indexPath string, cons *ImageConstraint, onlySigned bool) ([]*ImageMetadata, error) {
	params := simplestreams.ValueParams{
		DataType:      ImageIds,
		FilterFunc:    appendMatchingImages,
		ValueTemplate: ImageMetadata{},
	}
	items, err := simplestreams.GetMetadata(sources, indexPath, cons, onlySigned, params)
	if err != nil {
		return nil, err
	}
	metadata := make([]*ImageMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*ImageMetadata)
	}
	return metadata, nil
}

type imageKey struct {
	vtype   string
	arch    string
	storage string
}

// appendMatchingImages updates matchingImages with image metadata records from images which belong to the
// specified region. If an image already exists in matchingImages, it is not overwritten.
func appendMatchingImages(source simplestreams.DataSource, matchingImages []interface{},
	images map[string]interface{}, cons simplestreams.LookupConstraint) []interface{} {

	imagesMap := make(map[imageKey]*ImageMetadata, len(matchingImages))
	for _, val := range matchingImages {
		im := val.(*ImageMetadata)
		imagesMap[imageKey{im.VType, im.Arch, im.Storage}] = im
	}
	for _, val := range images {
		im := val.(*ImageMetadata)
		if cons.Params().Region != im.RegionName {
			continue
		}
		if _, ok := imagesMap[imageKey{im.VType, im.Arch, im.Storage}]; !ok {
			matchingImages = append(matchingImages, im)
		}
	}
	return matchingImages
}

// GetLatestImageIdMetadata is provided so it can be call by tests outside the imagemetadata package.
func GetLatestImageIdMetadata(data []byte, source simplestreams.DataSource, cons *ImageConstraint) ([]*ImageMetadata, error) {
	metadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", "<unknown>", ImageMetadata{})
	if err != nil {
		return nil, err
	}
	items, err := simplestreams.GetLatestMetadata(metadata, cons, source, appendMatchingImages)
	if err != nil {
		return nil, err
	}
	result := make([]*ImageMetadata, len(items))
	for i, md := range items {
		result[i] = md.(*ImageMetadata)
	}
	return result, nil
}
