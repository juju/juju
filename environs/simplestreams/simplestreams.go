// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The simplestreams package supports locating, parsing, and filtering metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
//
// Users of this package provide an empty struct and a matching function to be able to query and return a list
// of typed values for a given criteria.
package simplestreams

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
)

var logger = loggo.GetLogger("juju.environs.simplestreams")

// CloudSpec uniquely defines a specific cloud deployment.
type CloudSpec struct {
	Region   string
	Endpoint string
}

type LookupConstraint interface {
	// Generates a string array representing product ids or id gragments.
	// Ids are formed similarly to an ISCSI qualified name (IQN).
	// Since id's may be fragments corresponding to more than one product, matching is done
	// using regexp. Since id's may contain '.', these need to be quoted accordingly.
	Ids() ([]string, error)
	// Returns the constraint parameters.
	Params() LookupParams
}

// LookupParams defines criteria used to find a metadata record.
// Derived structs implement the Ids() method.
type LookupParams struct {
	CloudSpec
	Series string
	Arches []string
	// Stream can be "" for the default "released" stream, or "daily" for
	// daily images, or any other stream that the available simplestreams
	// metadata supports.
	Stream string
}

func (p LookupParams) Params() LookupParams {
	return p
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

func SeriesVersion(series string) (string, error) {
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

// The following structs define the data model used in the JSON metadata files.
// Not every model attribute is defined here, only the ones we care about.
// See the doc/README file in lp:simplestreams for more information.

// Metadata attribute values may point to a map of attribute values (aka aliases) and these attributes
// are used to override/augment the existing attributes.
type attributeValues map[string]string
type aliasesByAttribute map[string]attributeValues

// Exported for testing
type CloudMetadata struct {
	Products map[string]MetadataCatalog    `json:"products"`
	Aliases  map[string]aliasesByAttribute `json:"_aliases"`
	Updated  string                        `json:"updated"`
	Format   string                        `json:"format"`
}

type itemsByVersion map[string]*ItemCollection

type MetadataCatalog struct {
	Series     string         `json:"release"`
	Version    string         `json:"version"`
	Arch       string         `json:"arch"`
	RegionName string         `json:"region"`
	Endpoint   string         `json:"endpoint"`
	Items      itemsByVersion `json:"versions"`
}

// Exported for testing
type ItemCollection struct {
	Items      map[string]interface{} `json:"items"`
	Arch       string                 `json:"arch"`
	Version    string                 `json:"version"`
	RegionName string                 `json:"region"`
	Endpoint   string                 `json:"endpoint"`
}

// These structs define the model used for metadata indices.

type Indices struct {
	Indexes map[string]*IndexMetadata `json:"index"`
	Updated string                    `json:"updated"`
	Format  string                    `json:"format"`
}

// Exported for testing.
type IndexReference struct {
	Indices
	BaseURL     string
	valueParams ValueParams
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

// These structs define the model used to describe download mirrors.

type MirrorRefs struct {
	Mirrors map[string][]MirrorReference `json:"mirrors"`
	Updated string                       `json:"updated"`
	Format  string                       `json:"format"`
}

type MirrorReference struct {
	Updated  string      `json:"updated"`
	Format   string      `json:"format"`
	DataType string      `json:"datatype"`
	Path     string      `json:"path"`
	Clouds   []CloudSpec `json:"clouds"`
}

type MirrorMetadata struct {
	Updated string                  `json:"updated"`
	Format  string                  `json:"format"`
	Mirrors map[string][]MirrorInfo `json:"mirrors"`
}

type MirrorInfo struct {
	Clouds    []CloudSpec `json:"clouds"`
	MirrorURL string      `json:"mirror"`
	Path      string      `json:"path"`
}

type MirrorInfoSlice []MirrorInfo
type MirrorRefSlice []MirrorReference

// filter returns those entries from an MirrorInfo array for which the given
// match function returns true. It preserves order.
func (entries MirrorInfoSlice) filter(match func(*MirrorInfo) bool) MirrorInfoSlice {
	result := MirrorInfoSlice{}
	for _, mirrorInfo := range entries {
		if match(&mirrorInfo) {
			result = append(result, mirrorInfo)
		}
	}
	return result
}

// filter returns those entries from an MirrorInfo array for which the given
// match function returns true. It preserves order.
func (entries MirrorRefSlice) filter(match func(*MirrorReference) bool) MirrorRefSlice {
	result := MirrorRefSlice{}
	for _, mirrorRef := range entries {
		if match(&mirrorRef) {
			result = append(result, mirrorRef)
		}
	}
	return result
}

// extractCatalogsForProducts gives you just those catalogs from a
// cloudImageMetadata that are for the given product IDs.  They are kept in
// the order of the parameter.
func (metadata *CloudMetadata) extractCatalogsForProducts(productIds []string) []MetadataCatalog {
	result := []MetadataCatalog{}
	for id, catalog := range metadata.Products {
		for _, pid := range productIds {
			if productIdMatches(pid, id) {
				result = append(result, catalog)
			}
		}
	}
	return result
}

// extractIndexes returns just the array of indexes, in arbitrary order.
func (ind *Indices) extractIndexes() IndexMetadataSlice {
	result := make(IndexMetadataSlice, 0, len(ind.Indexes))
	for _, metadata := range ind.Indexes {
		result = append(result, metadata)
	}
	return result
}

// hasCloud tells you whether an IndexMetadata has the given cloud in its
// Clouds list. If IndexMetadata has no clouds defined, then hasCloud
// returns true regardless.
func (metadata *IndexMetadata) hasCloud(cloud CloudSpec) bool {
	for _, metadataCloud := range metadata.Clouds {
		if metadataCloud == cloud {
			return true
		}
	}
	return len(metadata.Clouds) == 0
}

// hasProduct tells you whether an IndexMetadata provides any of the given
// product IDs.
func (metadata *IndexMetadata) hasProduct(prodIds []string) bool {
	for _, pid := range metadata.ProductIds {
		if containsString(prodIds, pid) {
			return true
		}
	}
	return false
}

type IndexMetadataSlice []*IndexMetadata

// filter returns those entries from an IndexMetadata array for which the given
// match function returns true.  It preserves order.
func (entries IndexMetadataSlice) filter(match func(*IndexMetadata) bool) IndexMetadataSlice {
	result := IndexMetadataSlice{}
	for _, metadata := range entries {
		if match(metadata) {
			result = append(result, metadata)
		}
	}
	return result
}

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
	SignedSuffix     = ".sjson"
	UnsignedSuffix   = ".json"
)

type appendMatchingFunc func([]interface{}, map[string]interface{}, LookupConstraint) []interface{}

// ValueParams contains the information required to pull out from the metadata structs of a particular type.
type ValueParams struct {
	// The simplestreams data type key.
	DataType string
	// A function used to filter and return records of a given type.
	FilterFunc appendMatchingFunc
	// An struct representing the type of records to return.
	ValueTemplate interface{}
}

// urlJoin returns baseURL + relpath making sure to have a '/' inbetween them
// This doesn't try to do anything fancy with URL query or parameter bits
// It also doesn't use path.Join because that normalizes slashes, and you need
// to keep both slashes in 'http://'.
func urlJoin(baseURL, relpath string) string {
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + relpath
	}
	return baseURL + "/" + relpath
}

// GetMaybeSignedMetadata returns metadata records matching the specified constraint.
func GetMaybeSignedMetadata(baseURLs []string, indexPath string, cons LookupConstraint, requireSigned bool, params ValueParams) ([]interface{}, error) {
	var items []interface{}
	for _, baseURL := range baseURLs {
		indexURL := urlJoin(baseURL, indexPath)
		indexRef, err := GetIndexWithFormat(baseURL, indexPath, "index:1.0", requireSigned, params)
		if err != nil {
			if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
				logger.Debugf("cannot load index %q: %v", indexURL, err)
				continue
			}
			return nil, err
		}
		logger.Debugf("read metadata index at %q", indexURL)
		items, err = indexRef.getLatestMetadataWithFormat(cons, "products:1.0", requireSigned)
		if err != nil {
			if errors.IsNotFoundError(err) {
				logger.Debugf("skipping index because of error getting latest metadata %q: %v", indexURL, err)
				continue
			}
			return nil, err
		}
		if len(items) > 0 {
			break
		}
	}
	return items, nil
}

// GetMaybeSignedMirror returns a mirror info struct matching the specified content and cloud.
func GetMaybeSignedMirror(baseURLs []string, indexPath string, requireSigned bool, contentId string, cloudSpec CloudSpec) (*MirrorInfo, error) {
	var mirrorInfo *MirrorInfo
	for _, baseURL := range baseURLs {
		mirrorRefs, err := GetMirrorRefsWithFormat(baseURL, indexPath, "index:1.0", requireSigned)
		if err != nil {
			if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
				logger.Debugf("cannot load index %q: %v", urlJoin(baseURL, indexPath), err)
				continue
			}
			return nil, err
		}
		mirrorRef, err := mirrorRefs.GetMirrorReference(contentId, cloudSpec)
		if err != nil {
			if errors.IsNotFoundError(err) {
				logger.Debugf("skipping index because of error getting latest metadata %q: %v", urlJoin(baseURL, indexPath), err)
				continue
			}
			return nil, err
		}
		mirrorInfo, err = mirrorRef.getMirrorInfo(baseURL, contentId, cloudSpec, "mirrors:1.0", requireSigned)
		if err != nil {
			return nil, err
		}
	}
	if mirrorInfo == nil {
		return nil, errors.NotFoundf("mirror metadata for %q and cloud %v", contentId, cloudSpec)
	}
	return mirrorInfo, nil
}

// fetchData gets all the data from the given path relative to the given base URL.
// It returns the data found and the full URL used.
func fetchData(baseURL, relpath string, requireSigned bool) (data []byte, dataURL string, err error) {
	dataURL = urlJoin(baseURL, relpath)
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

// GetIndexWithFormat returns a simplestreams index of the specified format.
// Exported for testing.
func GetIndexWithFormat(baseURL, indexPath, format string, requireSigned bool, params ValueParams) (*IndexReference, error) {
	data, url, err := fetchData(baseURL, indexPath, requireSigned)
	if err != nil {
		if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices Indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata at URL %q: %v", url, err)
	}
	if indices.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q at URL %q", indices.Format, format, url)
	}
	return &IndexReference{
		BaseURL:     baseURL,
		Indices:     indices,
		valueParams: params,
	}, nil
}

// GetProductsPath returns the path to the metadata file containing products for the specified constraint.
// Exported for testing.
func (indexRef *IndexReference) GetProductsPath(cons LookupConstraint) (string, error) {
	prodIds, err := cons.Ids()
	if err != nil {
		return "", err
	}
	candidates := indexRef.extractIndexes()
	// Restrict to image-ids entries.
	dataTypeMatches := func(metadata *IndexMetadata) bool {
		return metadata.DataType == indexRef.valueParams.DataType
	}
	candidates = candidates.filter(dataTypeMatches)
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file missing %q data", indexRef.valueParams.DataType)
	}
	// Restrict by cloud spec.
	hasRightCloud := func(metadata *IndexMetadata) bool {
		return metadata.hasCloud(cons.Params().CloudSpec)
	}
	candidates = candidates.filter(hasRightCloud)
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file has no data for cloud %v", cons.Params().CloudSpec)
	}
	// Restrict by product IDs.
	hasProduct := func(metadata *IndexMetadata) bool {
		return metadata.hasProduct(prodIds)
	}
	candidates = candidates.filter(hasProduct)
	if len(candidates) == 0 {
		return "", errors.NotFoundf("index file has no data for product name(s) %q", prodIds)
	}

	logger.Debugf("candidate matches for products %q are %v", prodIds, candidates)

	// Pick arbitrary match.
	return candidates[0].ProductsFilePath, nil
}

// GetMirrorRefsWithFormat returns a simplestreams mirrors struct of the specified format.
// Exported for testing.
func GetMirrorRefsWithFormat(baseURL, indexPath, format string, requireSigned bool) (*MirrorRefs, error) {
	data, url, err := fetchData(baseURL, indexPath, requireSigned)
	if err != nil {
		if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var mirrorRefs MirrorRefs
	err = json.Unmarshal(data, &mirrorRefs)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON mirror metadata at URL %q: %v", url, err)
	}
	if mirrorRefs.Format != format {
		return nil, fmt.Errorf("unexpected mirror file format %q, expected %q at URL %q", mirrorRefs.Format, format, url)
	}
	return &mirrorRefs, nil
}

// extractMirrorRefs returns just the array of MirrorRef structs for the contentId, in arbitrary order.
func (mirrorRefs *MirrorRefs) extractMirrorRefs(contentId string) MirrorRefSlice {
	for id, refs := range mirrorRefs.Mirrors {
		if id == contentId {
			return refs
		}
	}
	return nil
}

// hasCloud tells you whether a MirrorReference has the given cloud in its
// Clouds list.
func (mirrorRef *MirrorReference) hasCloud(cloud CloudSpec) bool {
	for _, refCloud := range mirrorRef.Clouds {
		if refCloud == cloud {
			return true
		}
	}
	return false
}

// GetMirrorReference returns the reference to the metadata file containing mirrors for the specified content and cloud.
// Exported for testing.
func (mirrorRefs *MirrorRefs) GetMirrorReference(contentId string, cloud CloudSpec) (*MirrorReference, error) {
	candidates := mirrorRefs.extractMirrorRefs(contentId)
	if len(candidates) == 0 {
		return nil, errors.NotFoundf("mirror data for %q", contentId)
	}
	// Restrict by cloud spec.
	hasRightCloud := func(mirrorRef *MirrorReference) bool {
		return mirrorRef.hasCloud(cloud)
	}
	matchingCandidates := candidates.filter(hasRightCloud)
	if len(matchingCandidates) == 0 {
		// No cloud specific mirrors found so look for a non cloud specific mirror.
		for _, candidate := range candidates {
			if len(candidate.Clouds) == 0 {
				logger.Debugf("using default candidate for content id %q are %v", contentId, candidate)
				return &candidate, nil
			}
		}
		return nil, errors.NotFoundf("index file with cloud %v", cloud)
	}

	logger.Debugf("candidate matches for content id %q are %v", contentId, candidates)

	// Pick arbitrary match.
	return &matchingCandidates[0], nil
}

// getMirrorInfo returns mirror information from the mirror file at the given path for the specified content and cloud.
func (mirrorRef *MirrorReference) getMirrorInfo(baseURL, contentId string, cloud CloudSpec, format string, requireSigned bool) (*MirrorInfo, error) {
	metadata, err := GetMirrorMetadataWithFormat(baseURL, mirrorRef.Path, format, requireSigned)
	if err != nil {
		return nil, err
	}
	mirrorInfo, err := metadata.getMirrorInfo(contentId, cloud)
	if err != nil {
		return nil, err
	}
	return mirrorInfo, nil
}

// GetMirrorMetadataWithFormat returns simplestreams mirror data of the specified format.
// Exported for testing.
func GetMirrorMetadataWithFormat(baseURL, mirrorPath, format string, requireSigned bool) (*MirrorMetadata, error) {
	data, url, err := fetchData(baseURL, mirrorPath, requireSigned)
	if err != nil {
		if errors.IsNotFoundError(err) || errors.IsUnauthorizedError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read mirror data, %v", err)
	}
	var mirrors MirrorMetadata
	err = json.Unmarshal(data, &mirrors)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON mirror metadata at URL %q: %v", url, err)
	}
	if mirrors.Format != format {
		return nil, fmt.Errorf("unexpected mirror file format %q, expected %q at URL %q", mirrors.Format, format, url)
	}
	return &mirrors, nil
}

// hasCloud tells you whether an MirrorInfo has the given cloud in its
// Clouds list.
func (mirrorInfo *MirrorInfo) hasCloud(cloud CloudSpec) bool {
	for _, metadataCloud := range mirrorInfo.Clouds {
		if metadataCloud == cloud {
			return true
		}
	}
	return false
}

// getMirrorInfo returns the mirror metadata for the specified content and cloud.
func (mirrorMetadata *MirrorMetadata) getMirrorInfo(contentId string, cloud CloudSpec) (*MirrorInfo, error) {
	var candidates MirrorInfoSlice
	for id, m := range mirrorMetadata.Mirrors {
		if id == contentId {
			candidates = m
			break
		}
	}
	if len(candidates) == 0 {
		return nil, errors.NotFoundf("mirror info for %q", contentId)
	}

	// Restrict by cloud spec.
	hasRightCloud := func(mirrorInfo *MirrorInfo) bool {
		return mirrorInfo.hasCloud(cloud)
	}
	candidates = candidates.filter(hasRightCloud)
	if len(candidates) == 0 {
		return nil, errors.NotFoundf("mirror info with cloud %v", cloud)
	}

	// Pick arbitrary match.
	return &candidates[0], nil
}

// productIdMatches returns true if the product id expression equals or matches the product id.
func productIdMatches(matchExpr, productId string) bool {
	re, err := regexp.Compile("^" + matchExpr + "$")
	if err != nil {
		return false
	}
	return re.MatchString(productId)
}

// utility function to see if any of values matches element.
func containsString(values []string, element string) bool {
	for _, value := range values {
		if productIdMatches(value, element) {
			return true
		}
	}
	return false
}

// To keep the metadata concise, attributes on the metadata struct which have the same value for each
// item may be moved up to a higher level in the tree. denormaliseMetadata descends the tree
// and fills in any missing attributes with values from a higher level.
func (metadata *CloudMetadata) denormaliseMetadata() {
	for _, metadataCatalog := range metadata.Products {
		for _, ItemCollection := range metadataCatalog.Items {
			for _, item := range ItemCollection.Items {
				coll := *ItemCollection
				inherit(&coll, metadataCatalog)
				inherit(item, &coll)
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

// processAliases looks through the struct fields to see if
// any aliases apply, and sets attributes appropriately if so.
func (metadata *CloudMetadata) processAliases(item interface{}) {
	for tag := range tags(item) {
		aliases, ok := metadata.Aliases[tag]
		if !ok {
			continue
		}
		// We have found a set of aliases for one of the fields in the metadata struct.
		// Now check to see if the field matches one of the defined aliases.
		fields, ok := aliases[fieldByTag(item, tag)]
		if !ok {
			continue
		}
		// The alias matches - set all the aliased fields in the struct.
		for attr, val := range fields {
			setFieldByTag(item, attr, val, true)
		}
	}
}

// Apply any attribute aliases to the metadata records.
func (metadata *CloudMetadata) applyAliases() {
	for _, metadataCatalog := range metadata.Products {
		for _, ItemCollection := range metadataCatalog.Items {
			for _, item := range ItemCollection.Items {
				metadata.processAliases(item)
			}
		}
	}
}

// construct iterates over the metadata records and replaces the generic maps of values
// with structs of the required type.
func (metadata *CloudMetadata) construct(valueType reflect.Type) error {
	for _, metadataCatalog := range metadata.Products {
		for _, ItemCollection := range metadataCatalog.Items {
			for i, item := range ItemCollection.Items {
				val, err := structFromMap(valueType, item.(map[string]interface{}))
				if err != nil {
					return err
				}
				ItemCollection.Items[i] = val
			}
		}
	}
	return nil
}

// structFromMap marshalls a mapf of values into a metadata struct.
func structFromMap(valueType reflect.Type, attr map[string]interface{}) (interface{}, error) {
	data, err := json.Marshal(attr)
	if err != nil {
		return nil, err
	}
	val := reflect.New(valueType).Interface()
	err = json.Unmarshal([]byte(data), &val)
	return val, err
}

type structTags map[reflect.Type]map[string]int

var tagsForType structTags = make(structTags)

// RegisterStructTags ensures the json tags for the given structs are able to be used
// when parsing the simplestreams metadata.
func RegisterStructTags(vals ...interface{}) {
	tags := mkTags(vals...)
	for k, v := range tags {
		tagsForType[k] = v
	}
}

func init() {
	RegisterStructTags(MetadataCatalog{}, ItemCollection{})
}

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

// GetCloudMetadataWithFormat loads the entire cloud metadata encoded using the specified format.
// Exported for testing.
func (indexRef *IndexReference) GetCloudMetadataWithFormat(ic LookupConstraint, format string, requireSigned bool) (*CloudMetadata, error) {
	productFilesPath, err := indexRef.GetProductsPath(ic)
	if err != nil {
		return nil, err
	}
	logger.Debugf("finding products at path %q", productFilesPath)
	data, url, err := fetchData(indexRef.BaseURL, productFilesPath, requireSigned)
	if err != nil {
		return nil, fmt.Errorf("cannot read product data, %v", err)
	}
	return ParseCloudMetadata(data, format, url, indexRef.valueParams.ValueTemplate)
}

// ParseCloudMetadata parses the given bytes into simplestreams metadata.
func ParseCloudMetadata(data []byte, format, url string, valueTemplate interface{}) (*CloudMetadata, error) {
	var metadata CloudMetadata
	err := json.Unmarshal(data, &metadata)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON metadata at URL %q: %v", url, err)
	}
	if metadata.Format != format {
		return nil, fmt.Errorf("unexpected index file format %q, expected %q at URL %q", metadata.Format, format, url)
	}
	if valueTemplate != nil {
		err = metadata.construct(reflect.TypeOf(valueTemplate))
	}
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal JSON metadata at URL %q: %v", url, err)
	}
	metadata.applyAliases()
	metadata.denormaliseMetadata()
	return &metadata, nil
}

// getLatestMetadataWithFormat loads the metadata for the given cloud and orders the resulting structs
// starting with the most recent, and returns items which match the product criteria, choosing from the
// latest versions first.
func (indexRef *IndexReference) getLatestMetadataWithFormat(cons LookupConstraint, format string, requireSigned bool) ([]interface{}, error) {
	metadata, err := indexRef.GetCloudMetadataWithFormat(cons, format, requireSigned)
	if err != nil {
		return nil, err
	}
	return GetLatestMetadata(metadata, cons, indexRef.valueParams.FilterFunc)
}

// GetLatestMetadata extracts and returns the metadata records matching the given criteria.
func GetLatestMetadata(metadata *CloudMetadata, cons LookupConstraint, filterFunc appendMatchingFunc) ([]interface{}, error) {
	prodIds, err := cons.Ids()
	if err != nil {
		return nil, err
	}

	catalogs := metadata.extractCatalogsForProducts(prodIds)
	if len(catalogs) == 0 {
		availableProducts := make([]string, 0, len(metadata.Products))
		for product := range metadata.Products {
			availableProducts = append(availableProducts, product)
		}
		logger.Debugf("index has no records for product ids %v; it does have product ids %v", prodIds, availableProducts)
	}

	var matchingItems []interface{}
	for _, catalog := range catalogs {
		var bv byVersionDesc = make(byVersionDesc, len(catalog.Items))
		i := 0
		for vers, itemColl := range catalog.Items {
			bv[i] = collectionVersion{vers, itemColl}
			i++
		}
		sort.Sort(bv)
		for _, itemCollVersion := range bv {
			matchingItems = filterFunc(matchingItems, itemCollVersion.ItemCollection.Items, cons)
		}
	}
	return matchingItems, nil
}

type collectionVersion struct {
	version        string
	ItemCollection *ItemCollection
}

// byVersionDesc is used to sort a slice of collections in descending order of their
// version in YYYYMMDD.
type byVersionDesc []collectionVersion

func (bv byVersionDesc) Len() int { return len(bv) }
func (bv byVersionDesc) Swap(i, j int) {
	bv[i], bv[j] = bv[j], bv[i]
}
func (bv byVersionDesc) Less(i, j int) bool {
	return bv[i].version > bv[j].version
}
