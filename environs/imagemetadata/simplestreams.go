// // The imagemetadata package supports locating, parsing, and filtering Ubuntu image metadata in simplestreams format.
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

// Product spec is used to define the required characteristics of an Ubuntu image.
type ProductSpec struct {
	Release string
	Arch    string
	Stream  string // may be "", typically "release", "daily" etc
	// the name may be expensive to generate so cache it.
	cachedName string
}

// NewProductSpec creates a ProductSpec.
func NewProductSpec(release, arch, stream string) ProductSpec {
	return ProductSpec{
		Release: release,
		Arch:    arch,
		Stream:  stream,
	}
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

// Generates a string representing a product id formed similarly to an ISCSI qualified name (IQN).
func (ps *ProductSpec) Name() (string, error) {
	if ps.cachedName != "" {
		return ps.cachedName, nil
	}
	stream := ps.Stream
	if stream != "" {
		stream = "." + stream
	}
	// We need to find the release version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	err := updateDistroInfo()
	if err != nil {
		return "", err
	}
	if version, ok := releaseVersions[ps.Release]; ok {
		ps.cachedName = fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, ps.Arch)
		return ps.cachedName, nil
	}
	return "", fmt.Errorf("Invalid Ubuntu release %q", ps.Release)
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
func GetImageIdMetadata(baseURL, indexPath string, cloudSpec *CloudSpec, prodSpec *ProductSpec) ([]*ImageMetadata, error) {
	indexRef, err := getIndexWithFormat(baseURL, indexPath, "index:1.0")
	if err != nil {
		return nil, err
	}
	return indexRef.getLatestImageIdMetadataWithFormat(cloudSpec, prodSpec, "products:1.0")
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
func (indexRef *indexReference) getImageIdsPath(cloudSpec *CloudSpec, prodSpec *ProductSpec) (string, error) {
	prodSpecName, err := prodSpec.Name()
	if err != nil {
		return "", fmt.Errorf("cannot resolve Ubuntu version %q: %v", prodSpec.Release, err)
	}
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
			if pid == prodSpecName {
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
	for _, metadataCatalog := range metadata.Products {
		catalogAttrs := extractAttrMap(&metadataCatalog)
		for _, imageCollection := range metadataCatalog.Images {
			attrsToApply := make(map[string]string)
			for k, v := range catalogAttrs {
				attrsToApply[k] = v
			}
			collectionAttrs := extractAttrMap(imageCollection)
			for k, v := range collectionAttrs {
				if v != "" {
					attrsToApply[k] = v
				}
			}
			for _, im := range imageCollection.Images {
				applyAttributeValues(im, attrsToApply, false)
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
			for _, im := range imageCollection.Images {
				metadata.processAliases(im)
			}
		}
	}
}

type imageKey struct {
	vtype   string
	storage string
}

func findMatchingImages(matchingImages []*ImageMetadata, images map[string]*ImageMetadata, region string) []*ImageMetadata {
	imagesMap := make(map[imageKey]*ImageMetadata, len(matchingImages))
	for _, im := range matchingImages {
		imagesMap[imageKey{im.VType, im.Storage}] = im
	}
	for _, im := range images {
		if region != im.RegionName {
			continue
		}
		if _, ok := imagesMap[imageKey{im.VType, im.Storage}]; !ok {
			matchingImages = append(matchingImages, im)
		}
	}
	return matchingImages
}

// getCloudMetadataWithFormat loads the entire cloud image metadata encoded using the specified format.
func (indexRef *indexReference) getCloudMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) (*cloudImageMetadata, error) {
	productFilesPath, err := indexRef.getImageIdsPath(cloudSpec, prodSpec)
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
func (indexRef *indexReference) getLatestImageIdMetadataWithFormat(cloudSpec *CloudSpec, prodSpec *ProductSpec, format string) ([]*ImageMetadata, error) {
	imageMetadata, err := indexRef.getCloudMetadataWithFormat(cloudSpec, prodSpec, format)
	if err != nil {
		return nil, err
	}
	prodSpecName, err := prodSpec.Name()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve Ubuntu version %q: %v", prodSpec.Release, err)
	}
	metadataCatalog, ok := imageMetadata.Products[prodSpecName]
	if !ok {
		return nil, fmt.Errorf("no image metadata for %s", prodSpecName)
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
		matchingImages = findMatchingImages(matchingImages, imageCollVersion.imageCollection.Images, cloudSpec.Region)
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
