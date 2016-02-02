// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The tools package supports locating, parsing, and filtering Ubuntu tools metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package tools

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func init() {
	simplestreams.RegisterStructTags(ToolsMetadata{})
}

const (
	// ImageIds is the simplestreams tools content type.
	ContentDownload = "content-download"

	// StreamsVersionV1 is used to construct the path for accessing streams data.
	StreamsVersionV1 = "v1"

	// IndexFileVersion is used to construct the streams index file.
	IndexFileVersion = 2
)

var currentStreamsVersion = StreamsVersionV1

// This needs to be a var so we can override it for testing.
var DefaultBaseURL = "https://streams.canonical.com/juju/tools"

const (
	// Legacy release directory for Juju < 1.21.
	LegacyReleaseDirectory = "releases"

	// Used to specify the released tools metadata.
	ReleasedStream = "released"

	// Used to specify metadata for testing tools.
	TestingStream = "testing"

	// Used to specify the proposed tools metadata.
	ProposedStream = "proposed"

	// Used to specify the devel tools metadata.
	DevelStream = "devel"
)

// ToolsConstraint defines criteria used to find a tools metadata record.
type ToolsConstraint struct {
	simplestreams.LookupParams
	Version      version.Number
	MajorVersion int
	MinorVersion int
}

// NewVersionedToolsConstraint returns a ToolsConstraint for a tools with a specific version.
func NewVersionedToolsConstraint(vers version.Number, params simplestreams.LookupParams) *ToolsConstraint {
	return &ToolsConstraint{LookupParams: params, Version: vers}
}

// NewGeneralToolsConstraint returns a ToolsConstraint for tools with matching major/minor version numbers.
func NewGeneralToolsConstraint(majorVersion, minorVersion int, params simplestreams.LookupParams) *ToolsConstraint {
	return &ToolsConstraint{LookupParams: params, Version: version.Zero,
		MajorVersion: majorVersion, MinorVersion: minorVersion}
}

// IndexIds generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
func (tc *ToolsConstraint) IndexIds() []string {
	if tc.Stream == "" {
		return nil
	}
	return []string{ToolsContentId(tc.Stream)}
}

// ProductIds generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
func (tc *ToolsConstraint) ProductIds() ([]string, error) {
	var allIds []string
	for _, ser := range tc.Series {
		version, err := series.SeriesVersion(ser)
		if err != nil {
			if series.IsUnknownSeriesVersionError(err) {
				logger.Debugf("ignoring unknown series %q", ser)
				continue
			}
			return nil, err
		}
		ids := make([]string, len(tc.Arches))
		for i, arch := range tc.Arches {
			ids[i] = fmt.Sprintf("com.ubuntu.juju:%s:%s", version, arch)
		}
		allIds = append(allIds, ids...)
	}
	return allIds, nil
}

// ToolsMetadata holds information about a particular tools tarball.
type ToolsMetadata struct {
	Release  string `json:"release"`
	Version  string `json:"version"`
	Arch     string `json:"arch"`
	Size     int64  `json:"size"`
	Path     string `json:"path"`
	FullPath string `json:"-"`
	FileType string `json:"ftype"`
	SHA256   string `json:"sha256"`
}

func (t *ToolsMetadata) String() string {
	return fmt.Sprintf("%+v", *t)
}

// sortString is used by byVersion to sort a list of ToolsMetadata.
func (t *ToolsMetadata) sortString() string {
	return fmt.Sprintf("%v-%s-%s", t.Version, t.Release, t.Arch)
}

// binary returns the tools metadata's binary version, which may be used for
// map lookup.
func (t *ToolsMetadata) binary() (version.Binary, error) {
	num, err := version.Parse(t.Version)
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	return version.Binary{
		Number: num,
		Series: t.Release,
		Arch:   t.Arch,
	}, nil
}

func (t *ToolsMetadata) productId() (string, error) {
	seriesVersion, err := series.SeriesVersion(t.Release)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("com.ubuntu.juju:%s:%s", seriesVersion, t.Arch), nil
}

// Fetch returns a list of tools for the specified cloud matching the constraint.
// The base URL locations are as specified - the first location which has a file is the one used.
// Signed data is preferred, but if there is no signed data available and onlySigned is false,
// then unsigned data is used.
func Fetch(
	sources []simplestreams.DataSource, cons *ToolsConstraint,
) ([]*ToolsMetadata, *simplestreams.ResolveInfo, error) {

	params := simplestreams.GetMetadataParams{
		StreamsVersion:   currentStreamsVersion,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:        ContentDownload,
			FilterFunc:      appendMatchingTools,
			MirrorContentId: ToolsContentId(cons.Stream),
			ValueTemplate:   ToolsMetadata{},
		},
	}
	items, resolveInfo, err := simplestreams.GetMetadata(sources, params)
	if err != nil {
		return nil, nil, err
	}
	metadata := make([]*ToolsMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*ToolsMetadata)
	}
	// Sorting the metadata is not strictly necessary, but it ensures consistent ordering for
	// all compilers, and it just makes it easier to look at the data.
	Sort(metadata)
	return metadata, resolveInfo, nil
}

// Sort sorts a slice of ToolsMetadata in ascending order of their version
// in order to ensure the results of Fetch are ordered deterministically.
func Sort(metadata []*ToolsMetadata) {
	sort.Sort(byVersion(metadata))
}

type byVersion []*ToolsMetadata

func (b byVersion) Len() int           { return len(b) }
func (b byVersion) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byVersion) Less(i, j int) bool { return b[i].sortString() < b[j].sortString() }

// appendMatchingTools updates matchingTools with tools metadata records from tools which belong to the
// specified series. If a tools record already exists in matchingTools, it is not overwritten.
func appendMatchingTools(source simplestreams.DataSource, matchingTools []interface{},
	tools map[string]interface{}, cons simplestreams.LookupConstraint) ([]interface{}, error) {

	toolsMap := make(map[version.Binary]*ToolsMetadata, len(matchingTools))
	for _, val := range matchingTools {
		tm := val.(*ToolsMetadata)
		binary, err := tm.binary()
		if err != nil {
			return nil, errors.Trace(err)
		}
		toolsMap[binary] = tm
	}
	for _, val := range tools {
		tm := val.(*ToolsMetadata)
		if !set.NewStrings(cons.Params().Series...).Contains(tm.Release) {
			continue
		}
		if toolsConstraint, ok := cons.(*ToolsConstraint); ok {
			tmNumber := version.MustParse(tm.Version)
			if toolsConstraint.Version == version.Zero {
				if toolsConstraint.MajorVersion >= 0 && toolsConstraint.MajorVersion != tmNumber.Major {
					continue
				}
				if toolsConstraint.MinorVersion >= 0 && toolsConstraint.MinorVersion != tmNumber.Minor {
					continue
				}
			} else {
				if toolsConstraint.Version != tmNumber {
					continue
				}
			}
		}
		binary, err := tm.binary()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if _, ok := toolsMap[binary]; !ok {
			tm.FullPath, _ = source.URL(tm.Path)
			matchingTools = append(matchingTools, tm)
		}
	}
	return matchingTools, nil
}

type MetadataFile struct {
	Path string
	Data []byte
}

// MetadataFromTools returns a tools metadata list derived from the
// given tools list. The size and sha256 will not be computed if
// missing.
func MetadataFromTools(toolsList coretools.List, toolsDir string) []*ToolsMetadata {
	metadata := make([]*ToolsMetadata, len(toolsList))
	for i, t := range toolsList {
		path := fmt.Sprintf("%s/juju-%s-%s-%s.tgz", toolsDir, t.Version.Number, t.Version.Series, t.Version.Arch)
		metadata[i] = &ToolsMetadata{
			Release:  t.Version.Series,
			Version:  t.Version.Number.String(),
			Arch:     t.Version.Arch,
			Path:     path,
			FileType: "tar.gz",
			Size:     t.Size,
			SHA256:   t.SHA256,
		}
	}
	return metadata
}

// ResolveMetadata resolves incomplete metadata
// by fetching the tools from storage and computing
// the size and hash locally.
func ResolveMetadata(stor storage.StorageReader, toolsDir string, metadata []*ToolsMetadata) error {
	for _, md := range metadata {
		if md.Size != 0 {
			continue
		}
		binary, err := md.binary()
		if err != nil {
			return errors.Annotate(err, "cannot resolve metadata")
		}
		logger.Infof("Fetching tools from dir %q to generate hash: %v", toolsDir, binary)
		size, sha256hash, err := fetchToolsHash(stor, toolsDir, binary)
		// Older versions of Juju only know about ppc64, not ppc64el,
		// so if there's no metadata for ppc64, dd metadata for that arch.
		if errors.IsNotFound(err) && binary.Arch == arch.LEGACY_PPC64 {
			ppc64elBinary := binary
			ppc64elBinary.Arch = arch.PPC64EL
			md.Path = strings.Replace(md.Path, binary.Arch, ppc64elBinary.Arch, -1)
			size, sha256hash, err = fetchToolsHash(stor, toolsDir, ppc64elBinary)
		}
		if err != nil {
			return err
		}
		md.Size = size
		md.SHA256 = fmt.Sprintf("%x", sha256hash.Sum(nil))
	}
	return nil
}

// MergeMetadata merges the given tools metadata.
// If metadata for the same tools version exists in both lists,
// an entry with non-empty size/SHA256 takes precedence; if
// the two entries have different sizes/hashes, then an error is
// returned.
func MergeMetadata(tmlist1, tmlist2 []*ToolsMetadata) ([]*ToolsMetadata, error) {
	merged := make(map[version.Binary]*ToolsMetadata)
	for _, tm := range tmlist1 {
		binary, err := tm.binary()
		if err != nil {
			return nil, errors.Annotate(err, "cannot merge metadata")
		}
		merged[binary] = tm
	}
	for _, tm := range tmlist2 {
		binary, err := tm.binary()
		if err != nil {
			return nil, errors.Annotate(err, "cannot merge metadata")
		}
		if existing, ok := merged[binary]; ok {
			if tm.Size != 0 {
				if existing.Size == 0 {
					merged[binary] = tm
				} else if existing.Size != tm.Size || existing.SHA256 != tm.SHA256 {
					return nil, fmt.Errorf(
						"metadata mismatch for %s: sizes=(%v,%v) sha256=(%v,%v)",
						binary.String(),
						existing.Size, tm.Size,
						existing.SHA256, tm.SHA256,
					)
				}
			}
		} else {
			merged[binary] = tm
		}
	}
	list := make([]*ToolsMetadata, 0, len(merged))
	for _, metadata := range merged {
		list = append(list, metadata)
	}
	Sort(list)
	return list, nil
}

// ReadMetadata returns the tools metadata from the given storage for the specified stream.
func ReadMetadata(store storage.StorageReader, stream string) ([]*ToolsMetadata, error) {
	dataSource := storage.NewStorageSimpleStreamsDataSource("existing metadata", store, storage.BaseToolsPath, simplestreams.EXISTING_CLOUD_DATA, false)
	toolsConstraint, err := makeToolsConstraint(simplestreams.CloudSpec{}, stream, -1, -1, coretools.Filter{})
	if err != nil {
		return nil, err
	}
	metadata, _, err := Fetch([]simplestreams.DataSource{dataSource}, toolsConstraint)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return metadata, nil
}

// AllMetadataStreams is the set of streams for which there will be simplestreams tools metadata.
var AllMetadataStreams = []string{ReleasedStream, ProposedStream, TestingStream, DevelStream}

// ReadAllMetadata returns the tools metadata from the given storage for all streams.
// The result is a map of metadata slices, keyed on stream.
func ReadAllMetadata(store storage.StorageReader) (map[string][]*ToolsMetadata, error) {
	streamMetadata := make(map[string][]*ToolsMetadata)
	for _, stream := range AllMetadataStreams {
		metadata, err := ReadMetadata(store, stream)
		if err != nil {
			return nil, err
		}
		if len(metadata) == 0 {
			continue
		}
		streamMetadata[stream] = metadata
	}
	return streamMetadata, nil
}

// removeMetadataUpdated unmarshalls simplestreams metadata, clears the
// updated attribute, and then marshalls back to a string.
func removeMetadataUpdated(metadataBytes []byte) (string, error) {
	var metadata map[string]interface{}
	err := json.Unmarshal(metadataBytes, &metadata)
	if err != nil {
		return "", err
	}
	delete(metadata, "updated")

	metadataJson, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(metadataJson), nil
}

// metadataUnchanged returns true if the content of metadata for stream in stor is the same
// as generatedMetadata, ignoring the "updated" attribute.
func metadataUnchanged(stor storage.Storage, stream string, generatedMetadata []byte) (bool, error) {
	mdPath := ProductMetadataPath(stream)
	filePath := path.Join(storage.BaseToolsPath, mdPath)
	existingDataReader, err := stor.Get(filePath)
	// If the file can't be retrieved, consider it has changed.
	if err != nil {
		return false, nil
	}
	defer existingDataReader.Close()
	existingData, err := ioutil.ReadAll(existingDataReader)
	if err != nil {
		return false, err
	}

	// To do the comparison, we unmarshall the metadata, clear the
	// updated value, and marshall back to a string.
	existingMetadata, err := removeMetadataUpdated(existingData)
	if err != nil {
		return false, err
	}
	newMetadata, err := removeMetadataUpdated(generatedMetadata)
	if err != nil {
		return false, err
	}
	return existingMetadata == newMetadata, nil
}

// WriteMetadata writes the given tools metadata for the specified streams to the given storage.
// streamMetadata contains all known metadata so that the correct index files can be written.
// Only product files for the specified streams are written.
func WriteMetadata(stor storage.Storage, streamMetadata map[string][]*ToolsMetadata, streams []string, writeMirrors ShouldWriteMirrors) error {
	updated := time.Now()
	index, legacyIndex, products, err := MarshalToolsMetadataJSON(streamMetadata, updated)
	if err != nil {
		return err
	}
	metadataInfo := []MetadataFile{
		{simplestreams.UnsignedIndex(currentStreamsVersion, IndexFileVersion), index},
	}
	if legacyIndex != nil {
		metadataInfo = append(metadataInfo, MetadataFile{
			simplestreams.UnsignedIndex(currentStreamsVersion, 1), legacyIndex,
		})
	}
	for _, stream := range streams {
		if metadata, ok := products[stream]; ok {
			// If metadata hasn't changed, do not overwrite.
			unchanged, err := metadataUnchanged(stor, stream, metadata)
			if err != nil {
				return err
			}
			if unchanged {
				logger.Infof("Metadata for stream %q unchanged", stream)
				continue
			}
			// Metadata is different, so include it.
			metadataInfo = append(metadataInfo, MetadataFile{ProductMetadataPath(stream), metadata})
		}
	}
	if writeMirrors {
		streamsMirrorsMetadata := make(map[string][]simplestreams.MirrorReference)
		for stream := range streamMetadata {
			streamsMirrorsMetadata[ToolsContentId(stream)] = []simplestreams.MirrorReference{{
				Updated:  updated.Format("20060102"), // YYYYMMDD
				DataType: ContentDownload,
				Format:   simplestreams.MirrorFormat,
				Path:     simplestreams.MirrorFile,
			}}
		}
		mirrorsMetadata := map[string]map[string][]simplestreams.MirrorReference{
			"mirrors": streamsMirrorsMetadata,
		}
		mirrorsInfo, err := json.MarshalIndent(&mirrorsMetadata, "", "    ")
		if err != nil {
			return err
		}
		metadataInfo = append(
			metadataInfo, MetadataFile{simplestreams.UnsignedMirror(currentStreamsVersion), mirrorsInfo})
	}
	return writeMetadataFiles(stor, metadataInfo)
}

var writeMetadataFiles = func(stor storage.Storage, metadataInfo []MetadataFile) error {
	for _, md := range metadataInfo {
		filePath := path.Join(storage.BaseToolsPath, md.Path)
		logger.Infof("Writing %s", filePath)
		err := stor.Put(filePath, bytes.NewReader(md.Data), int64(len(md.Data)))
		if err != nil {
			return err
		}
	}
	return nil
}

type ShouldWriteMirrors bool

const (
	WriteMirrors      = ShouldWriteMirrors(true)
	DoNotWriteMirrors = ShouldWriteMirrors(false)
)

// MergeAndWriteMetadata reads the existing metadata from storage (if any),
// and merges it with metadata generated from the given tools list. The
// resulting metadata is written to storage.
func MergeAndWriteMetadata(stor storage.Storage, toolsDir, stream string, tools coretools.List, writeMirrors ShouldWriteMirrors) error {
	existing, err := ReadAllMetadata(stor)
	if err != nil {
		return err
	}
	metadata := MetadataFromTools(tools, toolsDir)
	if metadata, err = MergeMetadata(metadata, existing[stream]); err != nil {
		return err
	}
	existing[stream] = metadata
	return WriteMetadata(stor, existing, []string{stream}, writeMirrors)
}

// fetchToolsHash fetches the tools from storage and calculates
// its size in bytes and computes a SHA256 hash of its contents.
func fetchToolsHash(stor storage.StorageReader, stream string, ver version.Binary) (size int64, sha256hash hash.Hash, err error) {
	r, err := storage.Get(stor, StorageName(ver, stream))
	if err != nil {
		return 0, nil, err
	}
	defer r.Close()
	sha256hash = sha256.New()
	size, err = io.Copy(sha256hash, r)
	return size, sha256hash, err
}
