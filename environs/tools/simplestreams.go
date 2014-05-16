// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The tools package supports locating, parsing, and filtering Ubuntu tools metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package tools

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/version/ubuntu"
)

func init() {
	simplestreams.RegisterStructTags(ToolsMetadata{})
}

const (
	ContentDownload = "content-download"
)

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

// ToolsConstraint defines criteria used to find a tools metadata record.
type ToolsConstraint struct {
	simplestreams.LookupParams
	Version      version.Number
	MajorVersion int
	MinorVersion int
	Released     bool
}

// NewVersionedToolsConstraint returns a ToolsConstraint for a tools with a specific version.
func NewVersionedToolsConstraint(vers version.Number, params simplestreams.LookupParams) *ToolsConstraint {
	return &ToolsConstraint{LookupParams: params, Version: vers}
}

// NewGeneralToolsConstraint returns a ToolsConstraint for tools with matching major/minor version numbers.
func NewGeneralToolsConstraint(majorVersion, minorVersion int, released bool, params simplestreams.LookupParams) *ToolsConstraint {
	return &ToolsConstraint{LookupParams: params, Version: version.Zero,
		MajorVersion: majorVersion, MinorVersion: minorVersion, Released: released}
}

// Ids generates a string array representing product ids formed similarly to an ISCSI qualified name (IQN).
func (tc *ToolsConstraint) Ids() ([]string, error) {
	var allIds []string
	for _, series := range tc.Series {
		version, err := ubuntu.SeriesVersion(series)
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

// binary returns the tools metadata's binary version,
// which may be used for map lookup.
func (t *ToolsMetadata) binary() version.Binary {
	return version.Binary{
		Number: version.MustParse(t.Version),
		Series: t.Release,
		Arch:   t.Arch,
	}
}

func (t *ToolsMetadata) productId() (string, error) {
	seriesVersion, err := ubuntu.SeriesVersion(t.Release)
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
	sources []simplestreams.DataSource, indexPath string, cons *ToolsConstraint,
	onlySigned bool) ([]*ToolsMetadata, *simplestreams.ResolveInfo, error) {

	params := simplestreams.ValueParams{
		DataType:        ContentDownload,
		FilterFunc:      appendMatchingTools,
		MirrorContentId: ToolsContentId,
		ValueTemplate:   ToolsMetadata{},
		PublicKey:       simplestreamsToolsPublicKey,
	}
	items, resolveInfo, err := simplestreams.GetMetadata(sources, indexPath, cons, onlySigned, params)
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
func (b byVersion) Less(i, j int) bool { return b[i].binary().String() < b[j].binary().String() }

// appendMatchingTools updates matchingTools with tools metadata records from tools which belong to the
// specified series. If a tools record already exists in matchingTools, it is not overwritten.
func appendMatchingTools(source simplestreams.DataSource, matchingTools []interface{},
	tools map[string]interface{}, cons simplestreams.LookupConstraint) []interface{} {

	toolsMap := make(map[version.Binary]*ToolsMetadata, len(matchingTools))
	for _, val := range matchingTools {
		tm := val.(*ToolsMetadata)
		toolsMap[tm.binary()] = tm
	}
	for _, val := range tools {
		tm := val.(*ToolsMetadata)
		if !set.NewStrings(cons.Params().Series...).Contains(tm.Release) {
			continue
		}
		if toolsConstraint, ok := cons.(*ToolsConstraint); ok {
			tmNumber := version.MustParse(tm.Version)
			if toolsConstraint.Version == version.Zero {
				if toolsConstraint.Released && tmNumber.IsDev() {
					continue
				}
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
		if _, ok := toolsMap[tm.binary()]; !ok {
			tm.FullPath, _ = source.URL(tm.Path)
			matchingTools = append(matchingTools, tm)
		}
	}
	return matchingTools
}

type MetadataFile struct {
	Path string
	Data []byte
}

// MetadataFromTools returns a tools metadata list derived from the
// given tools list. The size and sha256 will not be computed if
// missing.
func MetadataFromTools(toolsList coretools.List) []*ToolsMetadata {
	metadata := make([]*ToolsMetadata, len(toolsList))
	for i, t := range toolsList {
		path := fmt.Sprintf("releases/juju-%s-%s-%s.tgz", t.Version.Number, t.Version.Series, t.Version.Arch)
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
func ResolveMetadata(stor storage.StorageReader, metadata []*ToolsMetadata) error {
	for _, md := range metadata {
		if md.Size != 0 {
			continue
		}
		binary := md.binary()
		logger.Infof("Fetching tools to generate hash: %v", binary)
		size, sha256hash, err := fetchToolsHash(stor, binary)
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
		merged[tm.binary()] = tm
	}
	for _, tm := range tmlist2 {
		binary := tm.binary()
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

// ReadMetadata returns the tools metadata from the given storage.
func ReadMetadata(store storage.StorageReader) ([]*ToolsMetadata, error) {
	dataSource := storage.NewStorageSimpleStreamsDataSource("existing metadata", store, storage.BaseToolsPath)
	toolsConstraint, err := makeToolsConstraint(simplestreams.CloudSpec{}, -1, -1, coretools.Filter{})
	if err != nil {
		return nil, err
	}
	metadata, _, err := Fetch(
		[]simplestreams.DataSource{dataSource}, simplestreams.DefaultIndexPath, toolsConstraint, false)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return metadata, nil
}

var PublicMirrorsInfo = `{
 "mirrors": {
  "com.ubuntu.juju:released:tools": [
     {
      "datatype": "content-download",
      "path": "streams/v1/cpc-mirrors.json",
      "updated": "{{updated}}",
      "format": "mirrors:1.0"
     }
  ]
 }
}
`

// WriteMetadata writes the given tools metadata to the given storage.
func WriteMetadata(stor storage.Storage, metadata []*ToolsMetadata, writeMirrors ShouldWriteMirrors) error {
	updated := time.Now()
	index, products, err := MarshalToolsMetadataJSON(metadata, updated)
	if err != nil {
		return err
	}
	metadataInfo := []MetadataFile{
		{simplestreams.UnsignedIndex, index},
		{ProductMetadataPath, products},
	}
	if writeMirrors {
		mirrorsUpdated := updated.Format("20060102") // YYYYMMDD
		mirrorsInfo := strings.Replace(PublicMirrorsInfo, "{{updated}}", mirrorsUpdated, -1)
		metadataInfo = append(metadataInfo, MetadataFile{simplestreams.UnsignedMirror, []byte(mirrorsInfo)})
	}
	for _, md := range metadataInfo {
		logger.Infof("Writing %s", "tools/"+md.Path)
		err = stor.Put(path.Join(storage.BaseToolsPath, md.Path), bytes.NewReader(md.Data), int64(len(md.Data)))
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
func MergeAndWriteMetadata(stor storage.Storage, tools coretools.List, writeMirrors ShouldWriteMirrors) error {
	existing, err := ReadMetadata(stor)
	if err != nil {
		return err
	}
	metadata := MetadataFromTools(tools)
	if metadata, err = MergeMetadata(metadata, existing); err != nil {
		return err
	}
	return WriteMetadata(stor, metadata, writeMirrors)
}

// fetchToolsHash fetches the tools from storage and calculates
// its size in bytes and computes a SHA256 hash of its contents.
func fetchToolsHash(stor storage.StorageReader, ver version.Binary) (size int64, sha256hash hash.Hash, err error) {
	r, err := storage.Get(stor, StorageName(ver))
	if err != nil {
		return 0, nil, err
	}
	defer r.Close()
	sha256hash = sha256.New()
	size, err = io.Copy(sha256hash, r)
	return size, sha256hash, err
}
