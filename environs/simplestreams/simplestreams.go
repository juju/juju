// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.environs.simplestreams")

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
func (spec *CloudSpec) equals(other *CloudSpec) (res bool) {
	// TODO(axw) 2016-04-11 #1568715
	// This is a hack to enable the new Azure provider to
	// work with the old simplestreams format. We should
	// update how the publishing is done so that we generate
	// with both the old Title Case style, and the new
	// lowercasewithoutwhitespace style.
	if canonicalRegionName(spec.Region) != canonicalRegionName(other.Region) {
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

func canonicalRegionName(region string) string {
	region = strings.Replace(region, " ", "", -1)
	return strings.ToLower(region)
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
	// IndexIds generates a string array representing index ids formed similarly to an ISCSI qualified name (IQN).
	IndexIds() []string
	// ProductIds generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
	ProductIds() ([]string, error)
	// Params returns the constraint parameters.
	Params() LookupParams
}

// LookupParams defines criteria used to find a metadata record.
// Derived structs implement the IndexIds() and ProductIds() method.
type LookupParams struct {
	CloudSpec
	Releases []string
	Arches   []string
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

// Metadata attribute values may point to a map of attribute values (aka
// aliases) and these attributes are used to override/augment the existing
// attributes.
type attributeValues map[string]string
type aliasesByAttribute map[string]attributeValues

type CloudMetadata struct {
	Products   map[string]MetadataCatalog    `json:"products"`
	Aliases    map[string]aliasesByAttribute `json:"_aliases,omitempty"`
	Updated    string                        `json:"updated"`
	Format     string                        `json:"format"`
	ContentId  string                        `json:"content_id"`
	RegionName string                        `json:"region,omitempty"`
	Endpoint   string                        `json:"endpoint,omitempty"`
	Storage    string                        `json:"root_store,omitempty"`
	VirtType   string                        `json:"virt,omitempty"`
}

type MetadataCatalog struct {
	Release    string `json:"release,omitempty"`
	Version    string `json:"version,omitempty"`
	Arch       string `json:"arch,omitempty"`
	RegionName string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Storage    string `json:"root_store,omitempty"`
	VirtType   string `json:"virt,omitempty"`

	// Items is a mapping from version to an ItemCollection,
	// where the version is the date the items were produced,
	// in the format YYYYMMDD.
	Items map[string]*ItemCollection `json:"versions"`
}

type ItemCollection struct {
	rawItems   map[string]*json.RawMessage
	Items      map[string]interface{} `json:"items"`
	Arch       string                 `json:"arch,omitempty"`
	Release    string                 `json:"release,omitempty"`
	Version    string                 `json:"version,omitempty"`
	RegionName string                 `json:"region,omitempty"`
	Endpoint   string                 `json:"endpoint,omitempty"`
	Storage    string                 `json:"root_store,omitempty"`
	VirtType   string                 `json:"virt,omitempty"`
}

// These structs define the model used for metadata indices.

type Indices struct {
	Indexes map[string]*IndexMetadata `json:"index"`
	Updated string                    `json:"updated"`
	Format  string                    `json:"format"`
}

// IndexReference is exported for testing.
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
	Clouds   []CloudSpec `json:"clouds,omitempty"`
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
	var result []MetadataCatalog
	for _, id := range productIds {
		if catalog, ok := metadata.Products[id]; ok {
			result = append(result, catalog)
		}
	}
	return result
}

// extractIndexes returns just the array of indexes, in arbitrary order.
func (ind *Indices) extractIndexes(indexIds []string) IndexMetadataSlice {
	result := make(IndexMetadataSlice, 0, len(ind.Indexes))
	if len(indexIds) == 0 {
		// No ids specified so return everything.
		for _, metadata := range ind.Indexes {
			result = append(result, metadata)
		}
	} else {
		// Return metadata for just the specified ids.
		for _, id := range indexIds {
			if metadata, ok := ind.Indexes[id]; ok {
				result = append(result, metadata)
			}
		}
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

// noMatchingProductsError is used to indicate that metadata files have been
// located, but there is no metadata satisfying a product criteria.
// It is used to distinguish from the situation where the metadata files could
// not be found.
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
	// These constants are used to specify the filenames used
	// when generating simplestreams metadata for local consumption
	// or for uploading to streams.canonical.com.
	unsignedIndex  = "streams/%s/index%s.json"
	unsignedMirror = "streams/%s/mirrors.json"
	MirrorFile     = "streams/v1/cpc-mirrors.json"

	// These constants are used when searching for simplestreams metadata.
	defaultLegacyIndexPath = "streams/%s/index"
	defaultIndexPath       = "streams/%s/index2"
	defaultMirrorsPath     = "streams/%s/mirrors"
	SignedSuffix           = ".sjson"
	UnsignedSuffix         = ".json"

	// These constants define the currently supported simplestreams data
	// formats.

	IndexFormat   = "index:1.0"
	ProductFormat = "products:1.0"
	MirrorFormat  = "mirrors:1.0"
)

// AppendMatchingFunc is the filter function signature used in our simple
// streams parsing code. It collects matching items from a DataSource in the
// returned interface slice.
type AppendMatchingFunc func(DataSource, []interface{}, map[string]interface{}, LookupConstraint) ([]interface{}, error)

// ValueParams contains the information required to pull out from the metadata
// structs of a particular type.
type ValueParams struct {
	// The simplestreams data type key.
	DataType string
	// The key to use when looking for content mirrors.
	MirrorContentId string
	// A function used to filter and return records of a given type.
	FilterFunc AppendMatchingFunc
	// An struct representing the type of records to return.
	ValueTemplate interface{}
}

// MirrorsPath returns the mirrors path for streamsVersion.
func MirrorsPath(streamsVersion string) string {
	return fmt.Sprintf(defaultMirrorsPath, streamsVersion)
}

// UnsignedIndex returns an unsigned index file name for streamsVersion.
func UnsignedIndex(streamsVersion string, indexFileVersion int) string {
	indexFileSuffix := ""
	if indexFileVersion > 1 {
		indexFileSuffix = fmt.Sprintf("%d", indexFileVersion)
	}
	return fmt.Sprintf(unsignedIndex, streamsVersion, indexFileSuffix)
}

// UnsignedMirror returns an unsigned mirror file name for streamsVersion.
func UnsignedMirror(streamsVersion string) string {
	return fmt.Sprintf(unsignedMirror, streamsVersion)
}

// Simplestreams defines a type for encapsulating the functionality for
// requesting simplestreams data.
//
// TODO (stickupkid): This requires more work. The idea is to localize all
// package level functions up on to this type. Passing the type through layers
// becomes simple.
type Simplestreams struct {
	factory DataSourceFactory
}

// NewSimpleStreams creates a new simplestreams accessor.
func NewSimpleStreams(factory DataSourceFactory) *Simplestreams {
	return &Simplestreams{
		factory: factory,
	}
}

// NewDataSource creates a new data source based on the config.
func (s Simplestreams) NewDataSource(config Config) DataSource {
	return s.factory.NewDataSource(config)
}

// GetMetadataParams defines parameters used to load simplestreams metadata.
type GetMetadataParams struct {
	StreamsVersion   string
	LookupConstraint LookupConstraint
	ValueParams      ValueParams
}

// GetMetadata returns metadata records matching the specified constraint,
// looking in each source for signed metadata. If onlySigned is false and no
// signed metadata is found in a source, the source is used to look for
// unsigned metadata.
// Each source is tried in turn until at least one signed (or unsigned) match
// is found.
func (s Simplestreams) GetMetadata(ctx context.Context, sources []DataSource, params GetMetadataParams) (items []interface{}, resolveInfo *ResolveInfo, err error) {
	for _, source := range sources {
		logger.Debugf(ctx, "searching for signed metadata in datasource %q", source.Description())
		items, resolveInfo, err = s.getMaybeSignedMetadata(ctx, source, params, true)
		// If no items are found using signed metadata, check unsigned.
		if err != nil && len(items) == 0 && !source.RequireSigned() {
			logger.Debugf(ctx, "falling back to search for unsigned metadata in datasource %q", source.Description())
			items, resolveInfo, err = s.getMaybeSignedMetadata(ctx, source, params, false)
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

// getMaybeSignedMetadata returns metadata records matching the specified constraint in params.
func (s Simplestreams) getMaybeSignedMetadata(ctx context.Context, source DataSource, params GetMetadataParams, signed bool) ([]interface{}, *ResolveInfo, error) {
	makeIndexPath := func(basePath string) string {
		pathNoSuffix := fmt.Sprintf(basePath, params.StreamsVersion)
		indexPath := pathNoSuffix + UnsignedSuffix
		if signed {
			indexPath = pathNoSuffix + SignedSuffix
		}
		return indexPath
	}

	resolveInfo := &ResolveInfo{}
	resolveInfo.Source = source.Description()
	resolveInfo.Signed = signed
	indexPath := makeIndexPath(defaultIndexPath)

	logger.Debugf(ctx, "looking for data index using path %s", indexPath)
	mirrorsPath := fmt.Sprintf(defaultMirrorsPath, params.StreamsVersion)
	cons := params.LookupConstraint

	indexRef, indexURL, err := s.fetchIndex(
		ctx,
		source,
		indexPath,
		mirrorsPath,
		cons.Params().CloudSpec,
		signed,
		params.ValueParams,
	)
	logger.Debugf(ctx, "looking for data index using URL %s", indexURL)
	if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
		legacyIndexPath := makeIndexPath(defaultLegacyIndexPath)
		logger.Debugf(ctx, "%s not accessed, actual error: %v", indexPath, errors.Details(err))
		logger.Debugf(ctx, "%s not accessed, trying legacy index path: %s", indexPath, legacyIndexPath)

		indexPath = legacyIndexPath
		indexRef, indexURL, err = s.fetchIndex(
			ctx,
			source,
			indexPath,
			mirrorsPath,
			cons.Params().CloudSpec,
			signed,
			params.ValueParams,
		)
	}
	resolveInfo.IndexURL = indexURL
	if err != nil {
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
			logger.Debugf(ctx, "cannot load index %q: %v", indexURL, err)
		}
		return nil, resolveInfo, err
	}
	logger.Debugf(ctx, "read metadata index at %q", indexURL)
	items, err := indexRef.getLatestMetadataWithFormat(ctx, cons, ProductFormat, signed)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			logger.Debugf(ctx, "skipping index %q because of missing information: %v", indexURL, err)
			return nil, resolveInfo, err
		}
		if _, ok := err.(*noMatchingProductsError); !ok {
			logger.Debugf(ctx, "%v", err)
		}
	}
	if indexRef.Source.Description() == "mirror" {
		resolveInfo.MirrorURL = indexRef.Source.(*urlDataSource).baseURL
	}
	return items, resolveInfo, err
}

// fetchIndex attempts to load the index file at indexPath in source.
func (s Simplestreams) fetchIndex(ctx context.Context, source DataSource, indexPath string, mirrorsPath string, cloudSpec CloudSpec,
	signed bool, params ValueParams) (indexRef *IndexReference, indexURL string, _ error) {
	indexURL, err := source.URL(indexPath)
	if err != nil {
		// Some providers return an error if asked for the URL of a non-existent
		// file. The best we can do is use the relative path for the URL when
		// logging messages.
		indexURL = indexPath
	}
	indexRef, err = s.GetIndexWithFormat(
		ctx,
		source,
		indexPath, IndexFormat,
		mirrorsPath,
		signed,
		cloudSpec,
		params,
	)
	return indexRef, indexURL, err
}

// fetchData gets all the data from the given source located at the specified
// path.
// It returns the data found and the full URL used.
func fetchData(ctx context.Context, source DataSource, path string, requireSigned bool) (data []byte, dataURL string, err error) {
	rc, dataURL, err := source.Fetch(ctx, path)
	if err != nil {
		logger.Tracef(ctx, "fetchData failed for %q: %v", dataURL, err)
		return nil, dataURL, err
	}
	defer func() { _ = rc.Close() }()
	if requireSigned {
		data, err = DecodeCheckSignature(rc, source.PublicSigningKey())
	} else {
		data, err = io.ReadAll(rc)
	}
	if err != nil {
		return nil, dataURL, errors.Annotatef(err, "cannot read data for source %q at URL %v", source.Description(), dataURL)
	}
	return data, dataURL, nil
}

// DataSourceFactory creates new DataSources for requesting index references.
type DataSourceFactory interface {
	// NewDataSource creates a new DataSource from a given config.
	NewDataSource(Config) DataSource
}

type defaultDataSourceFactory struct{}

// DefaultDataSourceFactory creates a generic new data source factory that
// creates new data sources based on the config.
func DefaultDataSourceFactory() DataSourceFactory {
	return defaultDataSourceFactory{}
}

func (defaultDataSourceFactory) NewDataSource(cfg Config) DataSource {
	return NewDataSource(cfg)
}

// GetIndexWithFormat returns a simplestreams index of the specified format.
// Exported for testing.
func (s Simplestreams) GetIndexWithFormat(
	ctx context.Context,
	source DataSource,
	indexPath, indexFormat, mirrorsPath string, requireSigned bool,
	cloudSpec CloudSpec, params ValueParams) (*IndexReference, error) {

	data, url, err := fetchData(ctx, source, indexPath, requireSigned)
	if err != nil {
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read index data, %v", err)
	}
	var indices Indices
	err = json.Unmarshal(data, &indices)
	if err != nil {
		logger.Errorf(ctx, "bad JSON index data at URL %q: %v", url, string(data))
		return nil, fmt.Errorf("cannot unmarshal JSON index metadata at URL %q: %v", url, err)
	}
	if indices.Format != indexFormat {
		return nil, fmt.Errorf(
			"unexpected index file format %q, expected %q at URL %q", indices.Format, indexFormat, url)
	}

	mirrors, url, err := getMirrorRefs(ctx, source, mirrorsPath, requireSigned)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.Unauthorized) {
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
			ctx, source, mirrors, params.DataType, params.MirrorContentId, cloudSpec, requireSigned)
		if err == nil {
			logger.Debugf(ctx, "using mirrored products path: %s", path.Join(mirrorInfo.MirrorURL, mirrorInfo.Path))
			indexRef.Source = s.factory.NewDataSource(Config{
				Description:          "mirror",
				BaseURL:              mirrorInfo.MirrorURL,
				PublicSigningKey:     source.PublicSigningKey(),
				HostnameVerification: true,
				Priority:             source.Priority(),
				RequireSigned:        requireSigned,
			})
			indexRef.MirroredProductsPath = mirrorInfo.Path
		} else {
			logger.Tracef(ctx, "no mirror information available for %s: %v", cloudSpec, err)
		}
	}

	return indexRef, nil
}

// getMirrorRefs parses and returns a simplestreams mirror reference.
func getMirrorRefs(ctx context.Context, source DataSource, baseMirrorsPath string, requireSigned bool) (MirrorRefs, string, error) {
	mirrorsPath := baseMirrorsPath + UnsignedSuffix
	if requireSigned {
		mirrorsPath = baseMirrorsPath + SignedSuffix
	}
	var mirrors MirrorRefs
	data, url, err := fetchData(ctx, source, mirrorsPath, requireSigned)
	if err != nil {
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
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
func getMirror(ctx context.Context, source DataSource, mirrors MirrorRefs, datatype, contentId string, cloudSpec CloudSpec,
	requireSigned bool) (*MirrorInfo, error) {

	mirrorRef, err := mirrors.getMirrorReference(datatype, contentId, cloudSpec)
	if err != nil {
		return nil, err
	}
	mirrorInfo, err := mirrorRef.getMirrorInfo(ctx, source, contentId, cloudSpec, MirrorFormat, requireSigned)
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
	prodIds, err := cons.ProductIds()
	if err != nil {
		return "", err
	}
	candidates := indexRef.extractIndexes(cons.IndexIds())
	// Restrict to the relevant data type entries.
	dataTypeMatches := func(metadata *IndexMetadata) bool {
		return metadata.DataType == indexRef.valueParams.DataType
	}
	candidates = candidates.filter(dataTypeMatches)
	if len(candidates) == 0 {
		// TODO: jam 2015-04-01 This isn't a great error to use,
		// because it is generally reserved for file-not-found
		// semantics.
		// This was formatted as: index file missing "content-download" data not found
		// It now formats as: "content-download" data not found
		// which at least reads better.
		// Shouldn't we be using noMatchingProductsError instead?
		return "", errors.NotFoundf("%q data", indexRef.valueParams.DataType)
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

	logger.Tracef(context.TODO(), "candidate matches for products %q are %v", prodIds, candidates)

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
				logger.Debugf(context.TODO(), "using default candidate for content id %q are %v", contentId, candidate)
				return &candidate, nil
			}
		}
		return nil, errors.NotFoundf("index file with cloud %v", cloud)
	}

	logger.Debugf(context.TODO(), "candidate matches for content id %q are %v", contentId, candidates)

	// Pick arbitrary match.
	return &matchingCandidates[0], nil
}

// getMirrorInfo returns mirror information from the mirror file at the given path for the specified content and cloud.
func (mirrorRef *MirrorReference) getMirrorInfo(ctx context.Context, source DataSource, contentId string, cloud CloudSpec, format string,
	requireSigned bool) (*MirrorInfo, error) {

	metadata, err := GetMirrorMetadataWithFormat(ctx, source, mirrorRef.Path, format, requireSigned)
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
func GetMirrorMetadataWithFormat(ctx context.Context, source DataSource, mirrorPath, format string,
	requireSigned bool) (*MirrorMetadata, error) {

	data, url, err := fetchData(ctx, source, mirrorPath, requireSigned)
	if err != nil {
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized) {
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
				inherit(&metadataCatalog, metadata)
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
	RegisterStructTags(CloudMetadata{}, MetadataCatalog{}, ItemCollection{})
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
func (indexRef *IndexReference) GetCloudMetadataWithFormat(ctx context.Context, cons LookupConstraint, format string, requireSigned bool) (*CloudMetadata, error) {
	productFilesPath, err := indexRef.GetProductsPath(cons)
	if err != nil {
		return nil, err
	}
	logger.Debugf(ctx, "finding products at path %q", productFilesPath)
	data, url, err := fetchData(ctx, indexRef.Source, productFilesPath, requireSigned)
	if err != nil {
		logger.Debugf(ctx, "can't read product data: %v", err)
		return nil, errors.Annotate(err, "cannot read product data")
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
		logger.Errorf(context.TODO(), "bad JSON product data at URL %q: %v", url, string(data))
		return nil, fmt.Errorf("cannot unmarshal JSON metadata at URL %q: %v", url, err)
	}
	metadata.applyAliases()
	metadata.denormaliseMetadata()
	return &metadata, nil
}

// getLatestMetadataWithFormat loads the metadata for the given cloud and orders the resulting structs
// starting with the most recent, and returns items which match the product criteria, choosing from the
// latest versions first.
func (indexRef *IndexReference) getLatestMetadataWithFormat(ctx context.Context, cons LookupConstraint, format string, requireSigned bool) ([]interface{}, error) {
	metadata, err := indexRef.GetCloudMetadataWithFormat(ctx, cons, format, requireSigned)
	if err != nil {
		return nil, err
	}
	logger.Tracef(ctx, "metadata: %v", metadata)
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
func GetLatestMetadata(metadata *CloudMetadata, cons LookupConstraint, source DataSource, filterFunc AppendMatchingFunc) ([]interface{}, error) {
	prodIds, err := cons.ProductIds()
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
			matchingItems, err = filterFunc(source, matchingItems, itemCollVersion.ItemCollection.Items, cons)
			if err != nil {
				return nil, errors.Trace(err)
			}
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

const SimplestreamsPublicKeyFile = "publicsimplestreamskey"

// UserPublicSigningKey returns the public signing key (if defined).
func UserPublicSigningKey() (string, error) {
	signingKeyFile := filepath.Join(agent.DefaultPaths.ConfDir, SimplestreamsPublicKeyFile)
	b, err := os.ReadFile(signingKeyFile)
	if os.IsNotExist(err) {
		// TODO (anastasiamac 2016-05-07)
		// We should not swallow this error
		// but propagate it up to the caller.
		// Bug #1579279
		return "", nil
	}
	if err != nil {
		return "", errors.Annotatef(err, "invalid public key file: %s", signingKeyFile)
	}
	return string(b), nil
}
