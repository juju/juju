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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.environs.simplestreams")

type ResolveInfo struct {
	Source    string `yaml:"source" json:"source"`
	Signed    bool   `yaml:"signed" json:"signed"`
	IndexURL  string `yaml:"indexURL" json:"indexURL"`
	MirrorURL string `yaml:"mirrorURL,omitempty" json:"mirrorURL,omitempty"`
}

// CloudSpec uniquely defines a specific cloud deployment.
type CloudSpec struct {
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
}

// equals returns true if spec == other, allowing for endpoints
// with or without a trailing "/".
func (spec *CloudSpec) equals(other *CloudSpec) bool {
	if spec.Region != other.Region {
		return false
	}
	specEndpoint := spec.Endpoint
	if !strings.HasSuffix(specEndpoint, "/") {
		specEndpoint += "/"
	}
	otherEndpoint := other.Endpoint
	if !strings.HasSuffix(otherEndpoint, "/") {
		otherEndpoint += "/"
	}
	return specEndpoint == otherEndpoint
}

// EmptyCloudSpec is used when we want all records regardless of cloud to be loaded.
var EmptyCloudSpec = CloudSpec{}

// HasRegion is implemented by instances which can provide a region to which they belong.
// A region is defined by region name and endpoint.
type HasRegion interface {
	// Region returns the necessary attributes to uniquely identify this cloud instance.
	// Currently these attributes are "region" and "endpoint" values.
	Region() (CloudSpec, error)
}

type LookupConstraint interface {
	// Generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
	Ids() ([]string, error)
	// Returns the constraint parameters.
	Params() LookupParams
}

// LookupParams defines criteria used to find a metadata record.
// Derived structs implement the Ids() method.
type LookupParams struct {
	CloudSpec
	Series []string
	Arches []string
	// Stream can be "" or "released" for the default "released" stream,
	// or "daily" for daily images, or any other stream that the available
	// simplestreams metadata supports.
	Stream string
}

func (p LookupParams) Params() LookupParams {
	return p
}

// The following structs define the data model used in the JSON metadata files.
// Not every model attribute is defined here, only the ones we care about.
// See the doc/README file in lp:simplestreams for more information.

// Metadata attribute values may point to a map of attribute values (aka aliases) and these attributes
// are used to override/augment the existing attributes.
type attributeValues map[string]string
type aliasesByAttribute map[string]attributeValues

type CloudMetadata struct {
	Products  map[string]MetadataCatalog    `json:"products"`
	Aliases   map[string]aliasesByAttribute `json:"_aliases,omitempty"`
	Updated   string                        `json:"updated"`
	Format    string                        `json:"format"`
	ContentId string                        `json:"content_id"`
}

type MetadataCatalog struct {
	Series     string `json:"release,omitempty"`
	Version    string `json:"version,omitempty"`
	Arch       string `json:"arch,omitempty"`
	RegionName string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`

	// Items is a mapping from version to an ItemCollection,
	// where the version is the date the items were produced,
	// in the format YYYYMMDD.
	Items map[string]*ItemCollection `json:"versions"`
}

type ItemCollection struct {
	rawItems   map[string]*json.RawMessage
	Items      map[string]interface{} `json:"items"`
	Arch       string                 `json:"arch,omitempty"`
	Series     string                 `json:"release,omitempty"`
	Version    string                 `json:"version,omitempty"`
	RegionName string                 `json:"region,omitempty"`
	Endpoint   string                 `json:"endpoint,omitempty"`
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
	MirroredProductsPath string
	Source               DataSource
	valueParams          ValueParams
}

type IndexMetadata struct {
	Updated          string      `json:"updated"`
	Format           string      `json:"format"`
	DataType         string      `json:"datatype"`
	CloudName        string      `json:"cloudname,omitempty"`
	Clouds           []CloudSpec `json:"clouds,omitempty"`
	ProductsFilePath string      `json:"path"`
	ProductIds       []string    `json:"products"`
}

// These structs define the model used to describe download mirrors.

type MirrorRefs struct {
	Mirrors map[string][]MirrorReference `json:"mirrors"`
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
	for _, id := range productIds {
		if catalog, ok := metadata.Products[id]; ok {
			result = append(result, catalog)
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

func (metadata *IndexMetadata) String() string {
	return fmt.Sprintf("%v", *metadata)
}

// hasCloud tells you whether an IndexMetadata has the given cloud in its
// Clouds list. If IndexMetadata has no clouds defined, then hasCloud
// returns true regardless so that the corresponding product records
// are searched.
func (metadata *IndexMetadata) hasCloud(cloud CloudSpec) bool {
	for _, metadataCloud := range metadata.Clouds {
		if metadataCloud.equals(&cloud) {
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

// noMatchingProductsError is used to indicate that metadata files have been located,
// but there is no metadata satisfying a product criteria.
// It is used to distinguish from the situation where the metadata files could not be found.
type noMatchingProductsError struct {
	msg string
}

func (e *noMatchingProductsError) Error() string {
	return e.msg
}

func newNoMatchingProductsError(message string, args ...interface{}) error {
	return &noMatchingProductsError{fmt.Sprintf(message, args...)}
}

const (
	StreamsDir       = "streams/v1"
	UnsignedIndex    = "streams/v1/index.json"
	DefaultIndexPath = "streams/v1/index"
	UnsignedMirror   = "streams/v1/mirrors.json"
	mirrorsPath      = "streams/v1/mirrors"
	signedSuffix     = ".sjson"
	UnsignedSuffix   = ".json"
)

type appendMatchingFunc func(DataSource, []interface{}, map[string]interface{}, LookupConstraint) []interface{}

// ValueParams contains the information required to pull out from the metadata structs of a particular type.
type ValueParams struct {
	// The simplestreams data type key.
	DataType string
	// The key to use when looking for content mirrors.
	MirrorContentId string
	// A function used to filter and return records of a given type.
	FilterFunc appendMatchingFunc
	// An struct representing the type of records to return.
	ValueTemplate interface{}
	// For signed metadata, the public key used to validate the signature.
	PublicKey string
}

// GetMetadata returns metadata records matching the specified constraint,looking in each source for signed metadata.
// If onlySigned is false and no signed metadata is found in a source, the source is used to look for unsigned metadata.
// Each source is tried in turn until at least one signed (or unsigned) match is found.
func GetMetadata(
	sources []DataSource, baseIndexPath string, cons LookupConstraint, onlySigned bool,
	params ValueParams) (items []interface{}, resolveInfo *ResolveInfo, err error) {
	for _, source := range sources {
		items, resolveInfo, err = getMaybeSignedMetadata(source, baseIndexPath, cons, true, params)
		// If no items are found using signed metadata, check unsigned.
		if err != nil && len(items) == 0 && !onlySigned {
			items, resolveInfo, err = getMaybeSignedMetadata(source, baseIndexPath, cons, false, params)
		}
		if err == nil {
			break
		}
	}
	if _, ok := err.(*noMatchingProductsError); ok {
		// no matching products is an internal error only
		err = nil
	}
	return items, resolveInfo, err
}

// getMaybeSignedMetadata returns metadata records matching the specified constraint.
func getMaybeSignedMetadata(source DataSource, baseIndexPath string, cons LookupConstraint,
	signed bool, params ValueParams) ([]interface{}, *ResolveInfo, error) {

	resolveInfo := &ResolveInfo{}
	indexPath := baseIndexPath + UnsignedSuffix
	if signed {
		indexPath = baseIndexPath + signedSuffix
	}
	var items []interface{}
	indexURL, err := source.URL(indexPath)
	if err != nil {
		// Some providers return an error if asked for the URL of a non-existent file.
		// So the best we can do is use the relative path for the URL when logging messages.
		indexURL = indexPath
	}
	resolveInfo.Source = source.Description()
	resolveInfo.Signed = signed
	resolveInfo.IndexURL = indexURL
	indexRef, err := GetIndexWithFormat(source, indexPath, "index:1.0", signed, cons.Params().CloudSpec, params)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsUnauthorized(err) {
			logger.Debugf("cannot load index %q: %v", indexURL, err)
		}
		return nil, resolveInfo, err
	}
	logger.Debugf("read metadata index at %q", indexURL)
	items, err = indexRef.getLatestMetadataWithFormat(cons, "products:1.0", signed)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("skipping index because of error getting latest metadata %q: %v", indexURL, err)
			return nil, resolveInfo, err
		}
		if _, ok := err.(*noMatchingProductsError); ok {
			logger.Debugf("%v", err)
		}
	}
	if indexRef.Source.Description() == "mirror" {
		resolveInfo.MirrorURL = indexRef.Source.(*urlDataSource).baseURL
	}
	return items, resolveInfo, err
}

// fetchData gets all the data from the given source located at the specified path.
// It returns the data found and the full URL used.
func fetchData(source DataSource, path string, requireSigned bool, publicKey string) (data []byte, dataURL string, err error) {
	rc, dataURL, err := source.Fetch(path)
	if err != nil {
		logger.Debugf("fetchData failed for %q: %v", dataURL, err)
		return nil, dataURL, errors.NotFoundf("invalid URL %q", dataURL)
	}
	defer rc.Close()
	if requireSigned {
		data, err = DecodeCheckSignature(rc, publicKey)
	} else {
		data, err = ioutil.ReadAll(rc)
	}
	if err != nil {
		return nil, dataURL, fmt.Errorf("cannot read URL data, %v", err)
	}
	return data, dataURL, nil
}

// GetIndexWithFormat returns a simplestreams index of the specified format.
// Exported for testing.
func GetIndexWithFormat(source DataSource, indexPath, indexFormat string, requireSigned bool,
	cloudSpec CloudSpec, params ValueParams) (*IndexReference, error) {

	data, url, err := fetchData(source, indexPath, requireSigned, params.PublicKey)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsUnauthorized(err) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices Indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		logger.Errorf("bad JSON index data at URL %q: %v", url, string(data))
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata at URL %q: %v", url, err)
	}
	if indices.Format != indexFormat {
		return nil, fmt.Errorf(
			"unexpected index file format %q, expected %q at URL %q", indices.Format, indexFormat, url)
	}

	mirrors, url, err := getMirrorRefs(source, mirrorsPath, requireSigned, params)
	if err != nil && !errors.IsNotFound(err) && !errors.IsUnauthorized(err) {
		return nil, fmt.Errorf("cannot load mirror metadata at URL %q: %v", url, err)
	}

	indexRef := &IndexReference{
		Source:      source,
		Indices:     indices,
		valueParams: params,
	}

	// Apply any mirror information to the source.
	if params.MirrorContentId != "" {
		mirrorInfo, err := getMirror(
			source, mirrors, params.DataType, params.MirrorContentId, cloudSpec, requireSigned, params.PublicKey)
		if err == nil {
			logger.Debugf("using mirrored products path: %s", path.Join(mirrorInfo.MirrorURL, mirrorInfo.Path))
			indexRef.Source = NewURLDataSource("mirror", mirrorInfo.MirrorURL, utils.VerifySSLHostnames)
			indexRef.MirroredProductsPath = mirrorInfo.Path
		} else {
			logger.Debugf("no mirror information available for %s: %v", cloudSpec, err)
		}
	}

	return indexRef, nil
}

// getMirrorRefs parses and returns a simplestreams mirror reference.
func getMirrorRefs(source DataSource, baseMirrorsPath string, requireSigned bool,
	params ValueParams) (MirrorRefs, string, error) {

	mirrorsPath := baseMirrorsPath + UnsignedSuffix
	if requireSigned {
		mirrorsPath = baseMirrorsPath + signedSuffix
	}
	var mirrors MirrorRefs
	data, url, err := fetchData(source, mirrorsPath, requireSigned, params.PublicKey)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsUnauthorized(err) {
			logger.Debugf("no mirror index file found")
			return mirrors, url, err
		}
		return mirrors, url, fmt.Errorf("cannot read mirrors data, %v", err)
	}
	err = json.Unmarshal(data, &mirrors)
	if err != nil {
		return mirrors, url, fmt.Errorf("cannot unmarshal JSON mirror metadata at URL %q: %v", url, err)
	}
	return mirrors, url, err
}

// getMirror returns a mirror info struct matching the specified content and cloud.
func getMirror(source DataSource, mirrors MirrorRefs, datatype, contentId string, cloudSpec CloudSpec,
	requireSigned bool, publicKey string) (*MirrorInfo, error) {

	mirrorRef, err := mirrors.getMirrorReference(datatype, contentId, cloudSpec)
	if err != nil {
		return nil, err
	}
	mirrorInfo, err := mirrorRef.getMirrorInfo(source, contentId, cloudSpec, "mirrors:1.0", requireSigned, publicKey)
	if err != nil {
		return nil, err
	}
	if mirrorInfo == nil {
		return nil, errors.NotFoundf("mirror metadata for %q and cloud %v", contentId, cloudSpec)
	}
	return mirrorInfo, nil
}

// GetProductsPath returns the path to the metadata file containing products for the specified constraint.
// Exported for testing.
func (indexRef *IndexReference) GetProductsPath(cons LookupConstraint) (string, error) {
	if indexRef.MirroredProductsPath != "" {
		return indexRef.MirroredProductsPath, nil
	}
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
	// Restrict by cloud spec, if required.
	if cons.Params().CloudSpec != EmptyCloudSpec {
		hasRightCloud := func(metadata *IndexMetadata) bool {
			return metadata.hasCloud(cons.Params().CloudSpec)
		}
		candidates = candidates.filter(hasRightCloud)
		if len(candidates) == 0 {
			return "", errors.NotFoundf("index file has no data for cloud %v", cons.Params().CloudSpec)
		}
	}
	// Restrict by product IDs.
	hasProduct := func(metadata *IndexMetadata) bool {
		return metadata.hasProduct(prodIds)
	}
	candidates = candidates.filter(hasProduct)
	if len(candidates) == 0 {
		return "", newNoMatchingProductsError("index file has no data for product name(s) %q", prodIds)
	}

	logger.Debugf("candidate matches for products %q are %v", prodIds, candidates)

	// Pick arbitrary match.
	return candidates[0].ProductsFilePath, nil
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
		if refCloud.equals(&cloud) {
			return true
		}
	}
	return false
}

// getMirrorReference returns the reference to the metadata file containing mirrors for the specified content and cloud.
func (mirrorRefs *MirrorRefs) getMirrorReference(datatype, contentId string, cloud CloudSpec) (*MirrorReference, error) {
	candidates := mirrorRefs.extractMirrorRefs(contentId)
	if len(candidates) == 0 {
		return nil, errors.NotFoundf("mirror data for %q", contentId)
	}
	// Restrict by cloud spec and datatype.
	hasRightCloud := func(mirrorRef *MirrorReference) bool {
		return mirrorRef.hasCloud(cloud) && mirrorRef.DataType == datatype
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
func (mirrorRef *MirrorReference) getMirrorInfo(source DataSource, contentId string, cloud CloudSpec, format string,
	requireSigned bool, publicKey string) (*MirrorInfo, error) {

	metadata, err := GetMirrorMetadataWithFormat(source, mirrorRef.Path, format, requireSigned, publicKey)
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
func GetMirrorMetadataWithFormat(source DataSource, mirrorPath, format string,
	requireSigned bool, publicKey string) (*MirrorMetadata, error) {

	data, url, err := fetchData(source, mirrorPath, requireSigned, publicKey)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsUnauthorized(err) {
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
		if metadataCloud.equals(&cloud) {
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

// utility function to see if element exists in values slice.
func containsString(values []string, element string) bool {
	for _, value := range values {
		if value == element {
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
			if err := ItemCollection.construct(valueType); err != nil {
				return err
			}
		}
	}
	return nil
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
func (indexRef *IndexReference) GetCloudMetadataWithFormat(cons LookupConstraint, format string, requireSigned bool) (*CloudMetadata, error) {
	productFilesPath, err := indexRef.GetProductsPath(cons)
	if err != nil {
		return nil, err
	}
	logger.Debugf("finding products at path %q", productFilesPath)
	data, url, err := fetchData(indexRef.Source, productFilesPath, requireSigned, indexRef.valueParams.PublicKey)
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
		logger.Errorf("bad JSON product data at URL %q: %v", url, string(data))
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
	logger.Debugf("metadata: %v", metadata)
	matches, err := GetLatestMetadata(metadata, cons, indexRef.Source, indexRef.valueParams.FilterFunc)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, newNoMatchingProductsError("index has no matching records")
	}
	return matches, nil
}

// GetLatestMetadata extracts and returns the metadata records matching the given criteria.
func GetLatestMetadata(metadata *CloudMetadata, cons LookupConstraint, source DataSource, filterFunc appendMatchingFunc) ([]interface{}, error) {
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
		return nil, newNoMatchingProductsError(
			"index has no records for product ids %v; it does have product ids %v", prodIds, availableProducts)
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
			matchingItems = filterFunc(source, matchingItems, itemCollVersion.ItemCollection.Items, cons)
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
