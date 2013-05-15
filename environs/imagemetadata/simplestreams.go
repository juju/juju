// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The imagemetadata package supports locating, parsing, and filtering Ubuntu image metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package imagemetadata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
)

// CloudSpec uniquely defines a specific cloud deployment.
type CloudSpec struct {
	Region   string
	Endpoint string
}

// ImageConstraint defines criteria used to find an image.
type ImageConstraint struct {
	CloudSpec
	Release string
	Arch    string
	Stream  string // may be "", typically "release", "daily" etc
	// the id may be expensive to generate so cache it.
	cachedId string
}

// NewImageConstraint creates a ImageConstraint.
func NewImageConstraint(region, endpoint, release, arch, stream string) ImageConstraint {
	return ImageConstraint{
		CloudSpec: CloudSpec{
			Endpoint: endpoint,
			Region:   region,
		},
		Release: release,
		Arch:    arch,
		Stream:  stream,
	}
}

// Generates a string representing a product id formed similarly to an ISCSI qualified name (IQN).
func (ic *ImageConstraint) Id() (string, error) {
	if ic.cachedId != "" {
		return ic.cachedId, nil
	}
	stream := ic.Stream
	if stream != "" {
		stream = "." + stream
	}
	// We need to find the release version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	err := updateDistroInfo()
	if err != nil {
		return "", err
	}
	if version, ok := releaseVersions[ic.Release]; ok {
		ic.cachedId = fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, ic.Arch)
		return ic.cachedId, nil
	}
	return "", fmt.Errorf("Invalid Ubuntu release %q", ic.Release)
}

// releaseVersions provides a mapping between Ubuntu series names and version numbers.
// The values here are current as of the time of writing. On Ubuntu systems, we update
// these values from /usr/share/distro-info/ubuntu.csv to ensure we have the latest values.
// On non-Ubuntu systems, these values provide a nice fallback option.
var releaseVersions = map[string]string{
	"precise": "12.04",
	"quantal": "12.10",
	"raring":  "13.04",
	"saucy":   "13.10",
}

// updateDistroInfo updates releaseVersions from /usr/share/distro-info/ubuntu.csv if possible..
func updateDistroInfo() error {
	// We need to find the release version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	f, err := os.Open("/usr/share/distro-info/ubuntu.csv")
	if err != nil {
		// On non-Ubuntu systems this file won't exist butr that's expected.
		return nil
	}
	defer f.Close()
	bufRdr := bufio.NewReader(f)
	for {
		line, err := bufRdr.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading distro info file file: %v", err)
		}
		// lines are of the form: "12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26"
		parts := strings.Split(line, ",")
		// the numeric version may contain a LTS moniker so strip that out.
		releaseInfo := strings.Split(parts[0], " ")
		releaseVersions[parts[2]] = releaseInfo[0]
	}
	return nil
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

type imagesByVersion map[string]*imageCollection

type imageMetadataCatalog struct {
	Release    string          `json:"release"`
	Version    string          `json:"version"`
	Arch       string          `json:"arch"`
	RegionName string          `json:"region"`
	Endpoint   string          `json:"endpoint"`
	Images     imagesByVersion `json:"versions"`
}

type imageCollection struct {
	Images     map[string]*ImageMetadata `json:"items"`
	RegionName string                    `json:"region"`
	Endpoint   string                    `json:"endpoint"`
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
	Indexes map[string]*indexMetadata `json:"index"`
	Updated string                    `json:"updated"`
	Format  string                    `json:"format"`
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
	DefaultIndexPath = "streams/v1/index.json"
	imageIds         = "image-ids"
)

// GetImageIdMetadata returns a list of images for the specified cloud matching the product criteria.
// The index file location is as specified.
func GetImageIdMetadata(baseURL, indexPath string, imageConstraint *ImageConstraint) ([]*ImageMetadata, error) {
	indexRef, err := getIndexWithFormat(baseURL, indexPath, "index:1.0")
	if err != nil {
		return nil, err
	}
	return indexRef.getLatestImageIdMetadataWithFormat(imageConstraint, "products:1.0")
}

// fetchData gets all the data from the given path relative to the given base URL.
func fetchData(baseURL, path string) ([]byte, string, error) {
	dataURL := baseURL
	if !strings.HasSuffix(dataURL, "/") {
		dataURL += "/"
	}
	dataURL += path
	resp, err := http.Get(dataURL)
	if err != nil {
		return nil, dataURL, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, dataURL, fmt.Errorf("cannot access URL %s, %s", dataURL, resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, dataURL, fmt.Errorf("cannot read URL data, %s", err.Error())
	}
	return data, dataURL, nil
}

func getIndexWithFormat(baseURL, indexPath, format string) (*indexReference, error) {
	data, url, err := fetchData(baseURL, indexPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata at URL %s: %s", url, err.Error())
	}
	if indices.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %s at URL %s", indices.Format, format, url)
	}
	return &indexReference{
		indices: indices,
		baseURL: baseURL,
	}, nil
}

// getImageIdsPath returns the path to the metadata file containing image ids for the specified
// cloud and product.
func (indexRef *indexReference) getImageIdsPath(imageConstraint *ImageConstraint) (string, error) {
	prodSpecId, err := imageConstraint.Id()
	if err != nil {
		return "", fmt.Errorf("cannot resolve Ubuntu version %q: %v", imageConstraint.Release, err)
	}
	var containsImageIds bool
	for _, metadata := range indexRef.Indexes {
		if metadata.DataType != imageIds {
			continue
		}
		containsImageIds = true
		var cloudSpecMatches bool
		for _, cs := range metadata.Clouds {
			if cs == imageConstraint.CloudSpec {
				cloudSpecMatches = true
				break
			}
		}
		var prodSpecMatches bool
		for _, pid := range metadata.ProductIds {
			if pid == prodSpecId {
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
	return "", fmt.Errorf("index file missing data for cloud %v", imageConstraint.CloudSpec)
}

// To keep the metadata concise, attributes on ImageMetadata which have the same value for each
// item may be moved up to a higher level in the tree. denormaliseImageMetadata descends the tree
// and fills in any missing attributes with values from a higher level.
func (metadata *cloudImageMetadata) denormaliseImageMetadata() {
	for _, metadataCatalog := range metadata.Products {
		for _, imageCollection := range metadataCatalog.Images {
			for _, im := range imageCollection.Images {
				coll := *imageCollection
				inherit(&coll, metadataCatalog)
				inherit(im, &coll)
			}
		}
	}
}

// inherit sets any blank fields in dst to their equivalent values in fields in src that have matching json tags.
// The dst parameter must be a pointer to a struct.
func inherit(dst, src interface{}) {
	for tag := range tags(dst) {
		setFieldByTag(dst, tag, fieldByTag(src, tag), false)
	}
}

// processAliases looks through the image fields to see if
// any aliases apply, and sets attributes appropriately
// if so.
func (metadata *cloudImageMetadata) processAliases(im *ImageMetadata) {
	for tag := range tags(im) {
		aliases, ok := metadata.Aliases[tag]
		if !ok {
			continue
		}
		// We have found a set of aliases for one of the fields in the image.
		// Now check to see if the field matches one of the defined aliases.
		fields, ok := aliases[fieldByTag(im, tag)]
		if !ok {
			continue
		}
		// The alias matches - set all the aliased fields in the image.
		for attr, val := range fields {
			setFieldByTag(im, attr, val, true)
		}
	}
}

// Apply any attribute aliases to the image metadata records.
func (metadata *cloudImageMetadata) applyAliases() {
	for _, metadataCatalog := range metadata.Products {
		for _, imageCollection := range metadataCatalog.Images {
			for _, im := range imageCollection.Images {
				metadata.processAliases(im)
			}
		}
	}
}

var tagsForType = mkTags(imageMetadataCatalog{}, imageCollection{}, ImageMetadata{})

func mkTags(vals ...interface{}) map[reflect.Type]map[string]int {
	typeMap := make(map[reflect.Type]map[string]int)
	for _, v := range vals {
		t := reflect.TypeOf(v)
		typeMap[t] = jsonTags(t)
	}
	return typeMap
}

// jsonTags returns a map from json tag to the field index for the string fields in the given type.
func jsonTags(t reflect.Type) map[string]int {
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("cannot get json tags on type %s", t))
	}
	tags := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != reflect.TypeOf("") {
			continue
		}
		if tag := f.Tag.Get("json"); tag != "" {
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[0:i]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				f.Name = tag
			}
		}
		tags[f.Name] = i
	}
	return tags
}

// tags returns the field offsets for the JSON tags defined by the given value, which must be
// a struct or a pointer to a struct.
func tags(x interface{}) map[string]int {
	t := reflect.TypeOf(x)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("expected struct, not %s", t))
	}

	if tagm := tagsForType[t]; tagm != nil {
		return tagm
	}
	panic(fmt.Errorf("%s not found in type table", t))
}

// fieldByTag returns the value for the field in x with the given JSON tag, or "" if there is no such field.
func fieldByTag(x interface{}, tag string) string {
	tagm := tags(x)
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if i, ok := tagm[tag]; ok {
		return v.Field(i).Interface().(string)
	}
	return ""
}

// setFieldByTag sets the value for the field in x with the given JSON tag to val.
// The override parameter specifies whether the value will be set even if the original value is non-empty.
func setFieldByTag(x interface{}, tag, val string, override bool) {
	i, ok := tags(x)[tag]
	if !ok {
		return
	}
	v := reflect.ValueOf(x).Elem()
	f := v.Field(i)
	if override || f.Interface().(string) == "" {
		f.Set(reflect.ValueOf(val))
	}
}

type imageKey struct {
	vtype   string
	storage string
}

// findMatchingImages updates matchingImages with image metadata records from images which belong to the
// specified region. If an image already exists in matchingImages, it is not overwritten.
func findMatchingImages(matchingImages []*ImageMetadata, images map[string]*ImageMetadata, imageConstraint *ImageConstraint) []*ImageMetadata {
	imagesMap := make(map[imageKey]*ImageMetadata, len(matchingImages))
	for _, im := range matchingImages {
		imagesMap[imageKey{im.VType, im.Storage}] = im
	}
	for _, im := range images {
		if imageConstraint.Region != im.RegionName {
			continue
		}
		if _, ok := imagesMap[imageKey{im.VType, im.Storage}]; !ok {
			matchingImages = append(matchingImages, im)
		}
	}
	return matchingImages
}

// getCloudMetadataWithFormat loads the entire cloud image metadata encoded using the specified format.
func (indexRef *indexReference) getCloudMetadataWithFormat(imageConstraint *ImageConstraint, format string) (*cloudImageMetadata, error) {
	productFilesPath, err := indexRef.getImageIdsPath(imageConstraint)
	if err != nil {
		return nil, fmt.Errorf("error finding product files path %s", err.Error())
	}
	data, url, err := fetchData(indexRef.baseURL, productFilesPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read product data, %v", err)
	}
	var imageMetadata cloudImageMetadata
	err = json.Unmarshal(data, &imageMetadata)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON image metadata at URL %s: %s", url, err.Error())
	}
	if imageMetadata.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q at URL %s", imageMetadata.Format, format, url)
	}
	imageMetadata.applyAliases()
	imageMetadata.denormaliseImageMetadata()
	return &imageMetadata, nil
}

// getLatestImageIdMetadataWithFormat loads the image metadata for the given cloud and order the images
// starting with the most recent, and returns images which match the product criteria, choosing from the
// latest versions first. The result is a list of images matching the criteria, but differing on type of storage etc.
func (indexRef *indexReference) getLatestImageIdMetadataWithFormat(imageConstraint *ImageConstraint, format string) ([]*ImageMetadata, error) {
	imageMetadata, err := indexRef.getCloudMetadataWithFormat(imageConstraint, format)
	if err != nil {
		return nil, err
	}
	prodSpecId, err := imageConstraint.Id()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve Ubuntu version %q: %v", imageConstraint.Release, err)
	}
	metadataCatalog, ok := imageMetadata.Products[prodSpecId]
	if !ok {
		return nil, fmt.Errorf("no image metadata for %s", prodSpecId)
	}
	var bv byVersionDesc = make(byVersionDesc, len(metadataCatalog.Images))
	i := 0
	for vers, imageColl := range metadataCatalog.Images {
		bv[i] = imageCollectionVersion{vers, imageColl}
		i++
	}
	sort.Sort(bv)
	var matchingImages []*ImageMetadata
	for _, imageCollVersion := range bv {
		matchingImages = findMatchingImages(matchingImages, imageCollVersion.imageCollection.Images, imageConstraint)
	}
	return matchingImages, nil
}

type imageCollectionVersion struct {
	version         string
	imageCollection *imageCollection
}

// byVersionDesc is used to sort a slice of image collections in descending order of their
// version in YYYYMMDD.
type byVersionDesc []imageCollectionVersion

func (bv byVersionDesc) Len() int { return len(bv) }
func (bv byVersionDesc) Swap(i, j int) {
	bv[i], bv[j] = bv[j], bv[i]
}
func (bv byVersionDesc) Less(i, j int) bool {
	return bv[i].version > bv[j].version
}
