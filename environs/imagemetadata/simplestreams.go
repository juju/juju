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
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
)

// CloudSpec uniquely defines a specific cloud deployment.
type CloudSpec struct {
	Region   string
	Endpoint string
}

var logger = loggo.GetLogger("juju.environs.imagemetadata")

// ImageConstraint defines criteria used to find an image.
type ImageConstraint struct {
	CloudSpec
	Series string
	Arches []string
	Stream string // may be "", typically "release", "daily" etc
}

// Generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
func (ic *ImageConstraint) Ids() ([]string, error) {
	stream := ic.Stream
	if stream != "" {
		stream = "." + stream
	}
	version, err := seriesVersion(ic.Series)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(ic.Arches))
	for i, arch := range ic.Arches {
		ids[i] = fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, arch)
	}
	return ids, nil
}

// seriesVersions provides a mapping between Ubuntu series names and version numbers.
// The values here are current as of the time of writing. On Ubuntu systems, we update
// these values from /usr/share/distro-info/ubuntu.csv to ensure we have the latest values.
// On non-Ubuntu systems, these values provide a nice fallback option.
var seriesVersions = map[string]string{
	"precise": "12.04",
	"quantal": "12.10",
	"raring":  "13.04",
	"saucy":   "13.10",
}

var (
	seriesVersionsMutex   sync.Mutex
	updatedseriesVersions bool
)

func seriesVersion(series string) (string, error) {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	if !updatedseriesVersions {
		err := updateDistroInfo()
		updatedseriesVersions = true
		if err != nil {
			return "", err
		}
	}
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	return "", fmt.Errorf("invalid series %q", series)
}

// updateDistroInfo updates seriesVersions from /usr/share/distro-info/ubuntu.csv if possible..
func updateDistroInfo() error {
	// We need to find the series version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	f, err := os.Open("/usr/share/distro-info/ubuntu.csv")
	if err != nil {
		// On non-Ubuntu systems this file won't exist but that's expected.
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
		// Ignore any malformed lines.
		if len(parts) < 3 {
			continue
		}
		// the numeric version may contain a LTS moniker so strip that out.
		seriesInfo := strings.Split(parts[0], " ")
		seriesVersions[parts[2]] = seriesInfo[0]
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
	Series     string          `json:"release"`
	Version    string          `json:"version"`
	Arch       string          `json:"arch"`
	RegionName string          `json:"region"`
	Endpoint   string          `json:"endpoint"`
	Images     imagesByVersion `json:"versions"`
}

type imageCollection struct {
	Images     map[string]*ImageMetadata `json:"items"`
	Arch       string                    `json:"arch"`
	RegionName string                    `json:"region"`
	Endpoint   string                    `json:"endpoint"`
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

// extractCatalogsForProducts gives you just those catalogs from a
// cloudImageMetadata that are for the given product IDs.  They are kept in
// the order of the parameter.
func (metadata *cloudImageMetadata) extractCatalogsForProducts(productIds []string) []imageMetadataCatalog {
	result := []imageMetadataCatalog{}
	for _, id := range productIds {
		if catalog, ok := metadata.Products[id]; ok {
			result = append(result, catalog)
		}
	}
	return result
}

// extractIndexes returns just the array of indexes, in arbitrary order.
func (ind *indices) extractIndexes() indexMetadataArray {
	result := indexMetadataArray{}
	for _, metadata := range ind.Indexes {
		result = append(result, metadata)
	}
	return result
}

// hasCloud tells you whether an indexMetadata has the given cloud in its
// Clouds list.
func (metadata *indexMetadata) hasCloud(cloud CloudSpec) bool {
	for _, metadataCloud := range metadata.Clouds {
		if metadataCloud == cloud {
			return true
		}
	}
	return false
}

// hasProduct tells you whether an indexMetadata provides any of the given
// product IDs.
func (metadata *indexMetadata) hasProduct(prodIds []string) bool {
	for _, pid := range metadata.ProductIds {
		if containsString(prodIds, pid) {
			return true
		}
	}
	return false
}

type indexMetadataArray []*indexMetadata

// filter returns those entries from an indexMetadata array for which the given
// match function returns "true."  It preserves order.
func (entries indexMetadataArray) filter(match func(*indexMetadata) bool) indexMetadataArray {
	result := indexMetadataArray{}
	for _, metadata := range entries {
		if match(metadata) {
			result = append(result, metadata)
		}
	}
	return result
}

// This needs to be a var so we can override it for testing.
var DefaultBaseURL = "http://cloud-images.ubuntu.com/releases"

var httpClient *http.Client = http.DefaultClient

// SetHttpClient replaces the default http.Client used to fetch the metadata
// and returns the old one.
func SetHttpClient(c *http.Client) *http.Client {
	old := httpClient
	httpClient = c
	return old
}

const (
	DefaultIndexPath = "streams/v1/index"
	signedSuffix     = ".sjson"
	unsignedSuffix   = ".json"
	imageIds         = "image-ids"
)

// Fetch returns a list of images for the specified cloud matching the constraint.
// The base URL locations are as specified - the first location which has a file is the one used.
// Signed data is preferred, but if there is no signed data available and onlySigned is false,
// then unsigned data is used.
func Fetch(baseURLs []string, indexPath string, ic *ImageConstraint, onlySigned bool) ([]*ImageMetadata, error) {
	metadata, err := getMaybeSignedImageIdMetadata(baseURLs, indexPath+signedSuffix, ic, true)
	if (err == nil && len(metadata) > 0) || onlySigned {
		return metadata, err
	}
	return getMaybeSignedImageIdMetadata(baseURLs, indexPath+unsignedSuffix, ic, false)
}

func getMaybeSignedImageIdMetadata(baseURLs []string, indexPath string, ic *ImageConstraint, requireSigned bool) ([]*ImageMetadata, error) {
	var metadata []*ImageMetadata
	for _, baseURL := range baseURLs {
		indexRef, err := getIndexWithFormat(baseURL, indexPath, "index:1.0", requireSigned)
		if err != nil {
			if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
				logger.Infof("cannot load index %q/%q: %v", baseURL, indexPath, err)
				continue
			}
			return nil, err
		}
		metadata, err = indexRef.getLatestImageIdMetadataWithFormat(ic, "products:1.0", requireSigned)
		if err != nil {
			if errors.IsNotFoundError(err) {
				logger.Infof("skipping index %q/%q: %v", baseURL, indexPath, err)
				continue
			}
			return nil, err
		}
		if len(metadata) > 0 {
			break
		}
	}
	return metadata, nil
}

// fetchData gets all the data from the given path relative to the given base URL.
// It returns the data found and the full URL used.
func fetchData(baseURL, path string, requireSigned bool) (data []byte, dataURL string, err error) {
	dataURL = baseURL
	if !strings.HasSuffix(dataURL, "/") {
		dataURL += "/"
	}
	dataURL += path
	resp, err := httpClient.Get(dataURL)
	if err != nil {
		return nil, dataURL, errors.NotFoundf("invalid URL %q", dataURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, dataURL, errors.NotFoundf("cannot find URL %q", dataURL)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, dataURL, errors.Unauthorizedf("unauthorised access to URL %q", dataURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, dataURL, fmt.Errorf("cannot access URL %q, %q", dataURL, resp.Status)
	}

	if requireSigned {
		data, err = DecodeCheckSignature(resp.Body)
	} else {
		data, err = ioutil.ReadAll(resp.Body)
	}
	if err != nil {
		return nil, dataURL, fmt.Errorf("cannot read URL data, %v", err)
	}
	return data, dataURL, nil
}

func getIndexWithFormat(baseURL, indexPath, format string, requireSigned bool) (*indexReference, error) {
	data, url, err := fetchData(baseURL, indexPath, requireSigned)
	if err != nil {
		if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata at URL %q: %v", url, err)
	}
	if indices.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q at URL %q", indices.Format, format, url)
	}
	return &indexReference{
		indices: indices,
		baseURL: baseURL,
	}, nil
}

// getImageIdsPath returns the path to the metadata file containing image ids the specified constraint.
func (indexRef *indexReference) getImageIdsPath(ic *ImageConstraint) (string, error) {
	prodIds, err := ic.Ids()
	if err != nil {
		return "", err
	}
	candidates := indexRef.extractIndexes()
	// Restrict to image-ids entries.
	candidates = candidates.filter(
		func(metadata *indexMetadata) bool { return metadata.DataType == imageIds })
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file missing %q data", imageIds)
	}
	// Restrict by cloud spec.
	candidates = candidates.filter(
		func(metadata *indexMetadata) bool { return metadata.hasCloud(ic.CloudSpec) })
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file has no data for cloud %v", ic.CloudSpec)
	}
	// Restrict by product IDs.
	candidates = candidates.filter(
		func(metadata *indexMetadata) bool { return metadata.hasProduct(prodIds) })
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file has no data for product name(s) %q", prodIds)
	}
	// Pick arbitrary match.
	return candidates[0].ProductsFilePath, nil
}

// utility function to see if element exists in values slice.
func containsString(values []string, element string) bool {
	for _, value := range values {
		if value == element {
			return true
		}
	}
	return false
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
// any aliases apply, and sets attributes appropriately if so.
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
	arch    string
	storage string
}

// appendMatchingImages updates matchingImages with image metadata records from images which belong to the
// specified region. If an image already exists in matchingImages, it is not overwritten.
func appendMatchingImages(matchingImages []*ImageMetadata, images map[string]*ImageMetadata, ic *ImageConstraint) []*ImageMetadata {
	imagesMap := make(map[imageKey]*ImageMetadata, len(matchingImages))
	for _, im := range matchingImages {
		imagesMap[imageKey{im.VType, im.Arch, im.Storage}] = im
	}
	for _, im := range images {
		if ic.Region != im.RegionName {
			continue
		}
		if _, ok := imagesMap[imageKey{im.VType, im.Arch, im.Storage}]; !ok {
			matchingImages = append(matchingImages, im)
		}
	}
	return matchingImages
}

// getCloudMetadataWithFormat loads the entire cloud image metadata encoded using the specified format.
func (indexRef *indexReference) getCloudMetadataWithFormat(ic *ImageConstraint, format string, requireSigned bool) (*cloudImageMetadata, error) {
	productFilesPath, err := indexRef.getImageIdsPath(ic)
	if err != nil {
		return nil, err
	}
	data, url, err := fetchData(indexRef.baseURL, productFilesPath, requireSigned)
	if err != nil {
		return nil, fmt.Errorf("cannot read product data, %v", err)
	}
	return parseCloudImageMetadata(data, format, url)
}

func parseCloudImageMetadata(data []byte, format, url string) (*cloudImageMetadata, error) {
	var imageMetadata cloudImageMetadata
	err := json.Unmarshal(data, &imageMetadata)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON image metadata at URL %q: %v", url, err)
	}
	if imageMetadata.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q at URL %q", imageMetadata.Format, format, url)
	}
	imageMetadata.applyAliases()
	imageMetadata.denormaliseImageMetadata()
	return &imageMetadata, nil
}

// getLatestImageIdMetadataWithFormat loads the image metadata for the given cloud and order the images
// starting with the most recent, and returns images which match the product criteria, choosing from the
// latest versions first. The result is a list of images matching the criteria, but differing on type of storage etc.
func (indexRef *indexReference) getLatestImageIdMetadataWithFormat(ic *ImageConstraint, format string, requireSigned bool) ([]*ImageMetadata, error) {
	imageMetadata, err := indexRef.getCloudMetadataWithFormat(ic, format, requireSigned)
	if err != nil {
		return nil, err
	}
	return getLatestImageIdMetadata(imageMetadata, ic)
}

func getLatestImageIdMetadata(imageMetadata *cloudImageMetadata, ic *ImageConstraint) ([]*ImageMetadata, error) {
	prodIds, err := ic.Ids()
	if err != nil {
		return nil, err
	}

	catalogs := imageMetadata.extractCatalogsForProducts(prodIds)
	if len(catalogs) == 0 {
		availableProducts := []string{}
		for product := range imageMetadata.Products {
			availableProducts = append(availableProducts, product)
		}
		logger.Debugf("index has no images for product ids %v; it does have product ids %v", prodIds, availableProducts)
	}

	var matchingImages []*ImageMetadata
	for _, catalog := range catalogs {
		var bv byVersionDesc = make(byVersionDesc, len(catalog.Images))
		i := 0
		for vers, imageColl := range catalog.Images {
			bv[i] = imageCollectionVersion{vers, imageColl}
			i++
		}
		sort.Sort(bv)
		for _, imageCollVersion := range bv {
			matchingImages = appendMatchingImages(matchingImages, imageCollVersion.imageCollection.Images, ic)
		}
	}
	return matchingImages, nil
}

// GetLatestImageIdMetadata is provided so it can be call by tests outside the simplestreams package.
func GetLatestImageIdMetadata(data []byte, ic *ImageConstraint) ([]*ImageMetadata, error) {
	imagemetadata, err := parseCloudImageMetadata(data, "products:1.0", "<unknown>")
	if err != nil {
		return nil, err
	}
	return getLatestImageIdMetadata(imagemetadata, ic)
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
