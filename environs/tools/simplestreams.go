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
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/juju/arch"
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

// simplestreamsToolsPublicKey is the public key required to
// authenticate the simple streams data on http://streams.canonical.com.
// Declared as a var so it can be overidden for testing.
var simplestreamsToolsPublicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.11 (GNU/Linux)

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
=hQAI
-----END PGP PUBLIC KEY BLOCK-----
`

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
	for _, series := range tc.Series {
		version, err := version.SeriesVersion(series)
		if err != nil {
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
// map lookup. It is possible for a binary to have an unkown OS.
func (t *ToolsMetadata) binary() (version.Binary, error) {
	num, err := version.Parse(t.Version)
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	toolsOS, err := version.GetOSFromSeries(t.Release)
	if err != nil && !version.IsUnknownOSForSeriesError(err) {
		return version.Binary{}, errors.Trace(err)
	}
	return version.Binary{
		Number: num,
		Series: t.Release,
		Arch:   t.Arch,
		OS:     toolsOS,
	}, nil
}

func (t *ToolsMetadata) productId() (string, error) {
	seriesVersion, err := version.SeriesVersion(t.Release)
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
	onlySigned bool) ([]*ToolsMetadata, *simplestreams.ResolveInfo, error) {

	params := simplestreams.GetMetadataParams{
		StreamsVersion:   currentStreamsVersion,
		OnlySigned:       onlySigned,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:        ContentDownload,
			FilterFunc:      appendMatchingTools,
			MirrorContentId: ToolsContentId(cons.Stream),
			ValueTemplate:   ToolsMetadata{},
			PublicKey:       simplestreamsToolsPublicKey,
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
	dataSource := storage.NewStorageSimpleStreamsDataSource("existing metadata", store, storage.BaseToolsPath)
	toolsConstraint, err := makeToolsConstraint(simplestreams.CloudSpec{}, stream, -1, -1, coretools.Filter{})
	if err != nil {
		return nil, err
	}
	metadata, _, err := Fetch(
		[]simplestreams.DataSource{dataSource}, toolsConstraint, false)
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
