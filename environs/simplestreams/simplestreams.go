// Support for locating, parsing, and filtering Ubuntu image metadata in simplestreams format.
// See http://launchpad.net/simplestreams

package simplestreams

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

// Generates a string representing a product id in a known format.
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

// These structs define the model used for image metadata.
type ImageMetadata struct {
	Id          string `json:"id"`
	Storage     string `json:"root_store"`
	VType       string `json:"virt"`
	RegionAlias string `json:"crsn"`
	RegionName  string `json:"region"`
	Endpoint    string `json:"endpoint"`
}

type ImageCollection struct {
	Images     map[string]ImageMetadata `json:"items"`
	RegionName string                   `json:"region"`
	Endpoint   string                   `json:"endpoint"`
}

type ImagesByVersion map[string]ImageCollection

type ImageMetadataCatalog struct {
	Release    string          `json:"release"`
	Version    string          `json:"version"`
	Arch       string          `json:"arch"`
	RegionName string          `json:"region"`
	Endpoint   string          `json:"endpoint"`
	Images     ImagesByVersion `json:"versions"`
}

type AttributeValues map[string]string
type AliasesByAttribute map[string]AttributeValues

type CloudImageMetadata struct {
	Products map[string]ImageMetadataCatalog `json:"products"`
	Aliases  map[string]AliasesByAttribute   `json:"_aliases"`
	Updated  string                          `json:"updated"`
	Format   string                          `json:"format"`
}

// These structs define the model used to image metadata indices.
type Indices struct {
	Indexes map[string]IndexMetadata `json:"index"`
	Updated string                   `json:"updated"`
	Format  string                   `json:"format"`
}

type IndexReference struct {
	Indices
	baseURL string
}

type IndexMetadata struct {
	Updated          string      `json:"updated"`
	Format           string      `json:"format"`
	DataType         string      `json:"datatype"`
	CloudName        string      `json:"cloudname"`
	Clouds           []CloudSpec `json:"clouds"`
	ProductsFilePath string      `json:"path"`
	ProductIds       []string    `json:"products"`
}

const (
	defaultIndexPath = "streams/v1/index.js"
	imageIds         = "image-ids"
)

// GetDefaultImageIdMetadata returns a list of images for the specified cloud matching the product criteria.
// An index file in the default location is used.
func GetDefaultImageIdMetadata(baseURL string, cloudSpec *CloudSpec, prodSpec *ProductSpec) ([]*ImageMetadata, error) {
	return GetImageIdMetadata(baseURL, defaultIndexPath, cloudSpec, prodSpec)
}

// GetImageIdMetadata returns a list of images for the specified cloud matching the product criteria.
// The index file location is as specified.
func GetImageIdMetadata(baseURL, indexPath string, cloudSpec *CloudSpec, prodSpec *ProductSpec) ([]*ImageMetadata, error) {
	indexRef, err := getIndexWithFormat(baseURL, indexPath, "index:1.0")
	if err != nil {
		return nil, err
	}
	return indexRef.getLatestImageIdMetadataWithFormat(cloudSpec, prodSpec, "products:1.0")
}

// Helper function to fetch data from a given http location.
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
		return nil, fmt.Errorf("invalid URL %s, %s", dataURL, resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read URL data, %s", err.Error())
	}
	return data, nil
}

func getIndexWithFormat(baseURL, indexPath, format string) (*IndexReference, error) {
	data, err := fetchData(baseURL, indexPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read index file, %s", err.Error())
	}
	var indices Indices
	if len(data) > 0 {
		err = json.Unmarshal(data, &indices)
		if err != nil {
			return nil, fmt.Errorf("cannot unmarshal JSON index metadata: %s", err.Error())
		}
	}
	if indices.Format != format {
		return nil, fmt.Errorf("expected index file format %s, got %s", format, indices.Format)
	}
	return &IndexReference{
		Indices: indices,
		baseURL: baseURL,
	}, nil
}

// getImageIdsPath returns the path to the metadata file containing image ids for the specified
// cloud and product.
func (indexRef *IndexReference) getImageIdsPath(cloudSpec *CloudSpec, prodSpec *ProductSpec) (string, error) {
	var metadata *IndexMetadata
	var containsImageIds bool
	for _, m := range indexRef.Indexes {
		if m.DataType != imageIds {
			continue
		}
		containsImageIds = true
		var cloudSpecMatches bool
		for _, cs := range m.Clouds {
			if cs == *cloudSpec {
				cloudSpecMatches = true
				break
			}
		}
		var prodSpecMatches bool
		for _, pid := range m.ProductIds {
			if pid == prodSpec.String() {
				prodSpecMatches = true
				break
			}
		}
		if cloudSpecMatches && prodSpecMatches {
			tmp := m
			metadata = &tmp
			break
		}
	}
	if metadata == nil {
		if !containsImageIds {
			return "", fmt.Errorf("index file missing %q data", imageIds)
		} else {
			return "", fmt.Errorf("index file missing data for cloud %v", cloudSpec)
		}
	}
	return metadata.ProductsFilePath, nil
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
func (metadata *CloudImageMetadata) denormaliseImageMetadata() {
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
func applyAttributeValues(im *ImageMetadata, aliases AttributeValues, override bool) {
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
func (metadata *CloudImageMetadata) processAliases(im *ImageMetadata) {
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
func (metadata *CloudImageMetadata) applyAliases() {
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

// Load the entire cloud image metadata encoded using the specified format.
// Denormalise the data (flatten the tree) and apply any aliases.
func (indexRef *IndexReference) getCloudMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) (*CloudImageMetadata, error) {
	productFilesPath, err := indexRef.getImageIdsPath(cloudSpec, prodSpec)
	if err != nil {
		return nil, fmt.Errorf("error finding product files path %s", err.Error())
	}
	data, err := fetchData(indexRef.baseURL, productFilesPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read product file, %s", err.Error())
	}
	var imageMetadata CloudImageMetadata
	if len(data) > 0 {
		err = json.Unmarshal(data, &imageMetadata)
		if err != nil {
			return nil, fmt.Errorf("cannot unmarshal JSON image metadata: %s", err.Error())
		}
	}
	if imageMetadata.Format != format {
		return nil, fmt.Errorf("expected index file format %s, got %s", format, imageMetadata.Format)
	}
	imageMetadata.applyAliases()
	imageMetadata.denormaliseImageMetadata()
	return &imageMetadata, nil
}

// Load the image metadata for the given cloud and order the images starting with the most recent.
// Return images which match the product criteria, choosing from the latest versions first.
// The result is a list of images matching the criteria, but differing on type of storage etc.
func (indexRef *IndexReference) getLatestImageIdMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) ([]*ImageMetadata, error) {
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
	bv.imageCollections = make([]ImageCollection, len(metadataCatalog.Images))
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
	imageCollections []ImageCollection
}

func (bv byVersionDesc) Len() int { return len(bv.imageCollections) }
func (bv byVersionDesc) Swap(i, j int) {
	bv.versions[i], bv.versions[j] = bv.versions[j], bv.versions[i]
	bv.imageCollections[i], bv.imageCollections[j] = bv.imageCollections[j], bv.imageCollections[i]
}
func (bv byVersionDesc) Less(i, j int) bool {
	return bv.versions[i] > bv.versions[j]
}
