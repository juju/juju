// // The imagemetadata package supports locating, parsing, and filtering Ubuntu image metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package imagemetadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

// CloudSpec uniquely defines a specific cloud deployment.
type CloudSpec struct {
	Region   string
	Endpoint string
}

// releaseVersions provides a mapping between Ubuntu series names and version numbers.
var releaseVersions = map[string]string{
	"precise": "12.04",
	"quantal": "12.10",
	"raring":  "13.04",
	"saucy":   "13.10",
}

// Product spec is used to define the required characteristics of an Ubuntu image.
type ProductSpec struct {
	Release string
	Arch    string
	Stream  string // may be "", typically "release", "daily" etc
}

// Generates a string representing a product id formed similarly to an ISCSI qualified name (IQN).
func (ps *ProductSpec) String() string {
	stream := ps.Stream
	if stream != "" {
		stream = "." + stream
	}
	if version, ok := releaseVersions[ps.Release]; ok {
		return fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, ps.Arch)
	}
	panic(fmt.Errorf("Invalid Ubuntu release %q", ps.Release))
}

// The following structs define the data model used in the JSON image metadata files.
// Not every model attribute is defined here, only the ones we care about.
// See the doc/README file in lp:simplestreams for more information.

// These structs define the model used for image metadata.

// ImageMetadata attribute values may point to a map of attribute values (aka aliases) and these attributes
// are used to override/augment the existing ImageMetadata attributes.
type attributeValues map[string]string
type aliasesByAttribute map[string]attributeValues

type cloudImageMetadata struct {
	Products map[string]imageMetadataCatalog `json:"products"`
	Aliases  map[string]aliasesByAttribute   `json:"_aliases"`
	Updated  string                          `json:"updated"`
	Format   string                          `json:"format"`
}

type imagesByVersion map[string]imageCollection

type imageMetadataCatalog struct {
	Release    string          `json:"release"`
	Version    string          `json:"version"`
	Arch       string          `json:"arch"`
	RegionName string          `json:"region"`
	Endpoint   string          `json:"endpoint"`
	Images     imagesByVersion `json:"versions"`
}

type imageCollection struct {
	Images     map[string]ImageMetadata `json:"items"`
	RegionName string                   `json:"region"`
	Endpoint   string                   `json:"endpoint"`
}

// This is the only struct we need to export. The goal of this package is to provide a list of
// ImageMetadata records matching the supplied region, arch etc.
type ImageMetadata struct {
	Id          string `json:"id"`
	Storage     string `json:"root_store"`
	VType       string `json:"virt"`
	RegionAlias string `json:"crsn"`
	RegionName  string `json:"region"`
	Endpoint    string `json:"endpoint"`
}

// These structs define the model used to image metadata indices.

type indices struct {
	Indexes map[string]indexMetadata `json:"index"`
	Updated string                   `json:"updated"`
	Format  string                   `json:"format"`
}

type indexReference struct {
	indices
	baseURL string
}

type indexMetadata struct {
	Updated          string      `json:"updated"`
	Format           string      `json:"format"`
	DataType         string      `json:"datatype"`
	CloudName        string      `json:"cloudname"`
	Clouds           []CloudSpec `json:"clouds"`
	ProductsFilePath string      `json:"path"`
	ProductIds       []string    `json:"products"`
}

const (
	DefaultIndexPath = "streams/v1/index.js"
	imageIds         = "image-ids"
)

// GetImageIdMetadata returns a list of images for the specified cloud matching the product criteria.
// The index file location is as specified.
func GetImageIdMetadata(baseURL, indexPath string, cloudSpec *CloudSpec, prodSpec *ProductSpec) ([]*ImageMetadata, error) {
	indexRef, err := getIndexWithFormat(baseURL, indexPath, "index:1.0")
	if err != nil {
		return nil, err
	}
	return indexRef.getLatestImageIdMetadataWithFormat(cloudSpec, prodSpec, "products:1.0")
}

// fetchData gets all the data from the given path relative to the given base URL.
func fetchData(baseURL, path string) ([]byte, error) {
	dataURL := baseURL
	if !strings.HasSuffix(dataURL, "/") {
		dataURL += "/"
	}
	dataURL += path
	resp, err := http.Get(dataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cannot access URL %s, %s", dataURL, resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read URL data, %s", err.Error())
	}
	return data, nil
}

func getIndexWithFormat(baseURL, indexPath, format string) (*indexReference, error) {
	data, err := fetchData(baseURL, indexPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata: %s", err.Error())
	}
	if indices.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %s", indices.Format, format)
	}
	return &indexReference{
		indices: indices,
		baseURL: baseURL,
	}, nil
}

// getImageIdsPath returns the path to the metadata file containing image ids for the specified
// cloud and product.
func (indexRef *indexReference) getImageIdsPath(cloudSpec *CloudSpec, prodSpec *ProductSpec) (string, error) {
	var containsImageIds bool
	for _, metadata := range indexRef.Indexes {
		if metadata.DataType != imageIds {
			continue
		}
		containsImageIds = true
		var cloudSpecMatches bool
		for _, cs := range metadata.Clouds {
			if cs == *cloudSpec {
				cloudSpecMatches = true
				break
			}
		}
		var prodSpecMatches bool
		for _, pid := range metadata.ProductIds {
			if pid == prodSpec.String() {
				prodSpecMatches = true
				break
			}
		}
		if cloudSpecMatches && prodSpecMatches {
			return metadata.ProductsFilePath, nil
		}
	}
	if !containsImageIds {
		return "", fmt.Errorf("index file missing %q data", imageIds)
	}
	return "", fmt.Errorf("index file missing data for cloud %v", cloudSpec)
}

// Convert a struct into a map of name, value pairs where the map keys are
// the json tags for each attribute.
func extractAttrMap(metadataStruct interface{}) map[string]string {
	attrs := make(map[string]string)
	v := reflect.ValueOf(metadataStruct).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Type().Kind() != reflect.String {
			continue
		}
		fieldTag := t.Field(i).Tag.Get("json")
		attrs[fieldTag] = f.String()
	}
	return attrs
}

// To keep the metadata concise, attributes on ImageMetadata which have the same value for each
// item may be moved up to a higher level in the tree. denormaliseImageMetadata descends the tree
// and fills in any missing attributes with values from a higher level.
func (metadata *cloudImageMetadata) denormaliseImageMetadata() {
	var attrsToApply map[string]string
	for _, metadataCatalog := range metadata.Products {
		attrsToApply = extractAttrMap(&metadataCatalog)
		for _, imageCollection := range metadataCatalog.Images {
			collectionAttrs := extractAttrMap(&imageCollection)
			for k, v := range collectionAttrs {
				if v != "" {
					attrsToApply[k] = v
				}
			}
			for id, im := range imageCollection.Images {
				applyAttributeValues(&im, attrsToApply, false)
				imageCollection.Images[id] = im
			}
		}
	}
}

// Apply the specified alias values to the image metadata record.
func applyAttributeValues(im *ImageMetadata, aliases attributeValues, override bool) {
	v := reflect.ValueOf(im).Elem()
	t := v.Type()
	for attrName, attrVale := range aliases {
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			fieldName := t.Field(i).Name
			fieldTag := t.Field(i).Tag.Get("json")
			if attrName != fieldName && attrName != fieldTag {
				continue
			}
			if override || f.String() == "" {
				f.SetString(attrVale)
			}
		}
	}
}

// Search the aliases map for aliases matching image attribute json tags and apply.
func (metadata *cloudImageMetadata) processAliases(im *ImageMetadata) {
	v := reflect.ValueOf(im).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		fieldTag := t.Field(i).Tag.Get("json")
		if aliases, ok := metadata.Aliases[fieldTag]; ok {
			for aliasValue, attrAliases := range aliases {
				if f.String() != aliasValue {
					continue
				}
				applyAttributeValues(im, attrAliases, true)
			}
		}
	}
}

// Apply any attribute aliases to the image metadata records.
func (metadata *cloudImageMetadata) applyAliases() {
	for _, metadataCatalog := range metadata.Products {
		for _, imageCollection := range metadataCatalog.Images {
			for id, im := range imageCollection.Images {
				metadata.processAliases(&im)
				imageCollection.Images[id] = im
			}
		}
	}
}

func findMatchingImages(matchingImages []*ImageMetadata, images map[string]ImageMetadata, region string) []*ImageMetadata {
	for _, val := range images {
		im := val
		if region != im.RegionName {
			continue
		}
		if !containsImage(matchingImages, &im) {
			matchingImages = append(matchingImages, &im)
		}
	}
	return matchingImages
}

func containsImage(images []*ImageMetadata, image *ImageMetadata) bool {
	for _, im := range images {
		if im.VType == image.VType && im.Storage == image.Storage {
			return true
		}
	}
	return false
}

// getCloudMetadataWithFormat loads the entire cloud image metadata encoded using the specified format.
func (indexRef *indexReference) getCloudMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) (*cloudImageMetadata, error) {
	productFilesPath, err := indexRef.getImageIdsPath(cloudSpec, prodSpec)
	if err != nil {
		return nil, fmt.Errorf("error finding product files path %s", err.Error())
	}
	data, err := fetchData(indexRef.baseURL, productFilesPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read product data, %v", err)
	}
	var imageMetadata cloudImageMetadata
	err = json.Unmarshal(data, &imageMetadata)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON image metadata: %s", err.Error())
	}
	if imageMetadata.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q", imageMetadata.Format, format)
	}
	imageMetadata.applyAliases()
	imageMetadata.denormaliseImageMetadata()
	return &imageMetadata, nil
}

// getLatestImageIdMetadataWithFormat loads the image metadata for the given cloud and order the images
// starting with the most recent, and returns images which match the product criteria, choosing from the
// latest versions first. The result is a list of images matching the criteria, but differing on type of storage etc.
func (indexRef *indexReference) getLatestImageIdMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) ([]*ImageMetadata, error) {
	imageMetadata, err := indexRef.getCloudMetadataWithFormat(cloudSpec, prodSpec, format)
	if err != nil {
		return nil, err
	}
	metadataCatalog, ok := imageMetadata.Products[prodSpec.String()]
	if !ok {
		return nil, fmt.Errorf("no image metadata for %s", prodSpec.String())
	}
	bv := byVersionDesc{}
	bv.versions = make([]string, len(metadataCatalog.Images))
	bv.imageCollections = make([]imageCollection, len(metadataCatalog.Images))
	i := 0
	for vers, imageColl := range metadataCatalog.Images {
		bv.versions[i] = vers
		bv.imageCollections[i] = imageColl
		i++
	}
	sort.Sort(bv)
	var matchingImages []*ImageMetadata
	for _, imageCollection := range bv.imageCollections {
		matchingImages = findMatchingImages(matchingImages, imageCollection.Images, cloudSpec.Region)
	}
	return matchingImages, nil
}

// byVersion is used to sort a slice of image collections as a side effect of
// sorting a matching slice of versions in YYYYMMDD.
type byVersionDesc struct {
	versions         []string
	imageCollections []imageCollection
}

func (bv byVersionDesc) Len() int { return len(bv.imageCollections) }
func (bv byVersionDesc) Swap(i, j int) {
	bv.versions[i], bv.versions[j] = bv.versions[j], bv.versions[i]
	bv.imageCollections[i], bv.imageCollections[j] = bv.imageCollections[j], bv.imageCollections[i]
}
func (bv byVersionDesc) Less(i, j int) bool {
	return bv.versions[i] > bv.versions[j]
}
