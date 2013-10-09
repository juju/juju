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
	"net/http"
	"strings"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

func init() {
	simplestreams.RegisterStructTags(ToolsMetadata{})
}

const (
	ContentDownload = "content-download"
	MirrorContentId = "com.ubuntu.juju:released:tools"
)

// simplestreamsToolsPublicKey is the public key required to
// authenticate the simple streams data on http://juju.canonical.com.
// Declared as a var so it can be overidden for testing.
var simplestreamsToolsPublicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.11 (GNU/Linux)

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
=hQAI
-----END PGP PUBLIC KEY BLOCK-----
`

// This needs to be a var so we can override it for testing.
var DefaultBaseURL = "https://juju.canonical.com/tools"

// ToolsConstraint defines criteria used to find a tools metadata record.
type ToolsConstraint struct {
	simplestreams.LookupParams
	Version      version.Number
	MajorVersion int
	MinorVersion int
	Released     bool
}

// NewVersionedToolsConstraint returns a ToolsConstraint for a tools with a specific version.
func NewVersionedToolsConstraint(vers string, params simplestreams.LookupParams) *ToolsConstraint {
	versNum := version.MustParse(vers)
	return &ToolsConstraint{LookupParams: params, Version: versNum}
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
		version, err := simplestreams.SeriesVersion(series)
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
	FullPath string `json:"-,omitempty"`
	FileType string `json:"ftype"`
	SHA256   string `json:"sha256"`
}

func (t *ToolsMetadata) String() string {
	return fmt.Sprintf("%+v", *t)
}

func (t *ToolsMetadata) productId() (string, error) {
	seriesVersion, err := simplestreams.SeriesVersion(t.Release)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("com.ubuntu.juju:%s:%s", seriesVersion, t.Arch), nil
}

func excludeDefaultSource(sources []simplestreams.DataSource) []simplestreams.DataSource {
	var result []simplestreams.DataSource
	for _, source := range sources {
		url, _ := source.URL("")
		if !strings.HasPrefix(url, "https://juju.canonical.com/tools") {
			result = append(result, source)
		}
	}
	return result
}

// Fetch returns a list of tools for the specified cloud matching the constraint.
// The base URL locations are as specified - the first location which has a file is the one used.
// Signed data is preferred, but if there is no signed data available and onlySigned is false,
// then unsigned data is used.
func Fetch(sources []simplestreams.DataSource, indexPath string, cons *ToolsConstraint, onlySigned bool) ([]*ToolsMetadata, error) {

	// TODO (wallyworld): 2013-09-05 bug 1220965
	// Until the official tools repository is set up, we don't want to use it.
	sources = excludeDefaultSource(sources)

	params := simplestreams.ValueParams{
		DataType:        ContentDownload,
		FilterFunc:      appendMatchingTools,
		MirrorContentId: MirrorContentId,
		ValueTemplate:   ToolsMetadata{},
		PublicKey:       simplestreamsToolsPublicKey,
	}
	items, err := simplestreams.GetMetadata(sources, indexPath, cons, onlySigned, params)
	if err != nil {
		return nil, err
	}
	metadata := make([]*ToolsMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*ToolsMetadata)
	}
	return metadata, nil
}

// appendMatchingTools updates matchingTools with tools metadata records from tools which belong to the
// specified series. If a tools record already exists in matchingTools, it is not overwritten.
func appendMatchingTools(source simplestreams.DataSource, matchingTools []interface{},
	tools map[string]interface{}, cons simplestreams.LookupConstraint) []interface{} {

	toolsMap := make(map[string]*ToolsMetadata, len(matchingTools))
	for _, val := range matchingTools {
		tm := val.(*ToolsMetadata)
		toolsMap[fmt.Sprintf("%s-%s-%s", tm.Release, tm.Version, tm.Arch)] = tm
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
		if _, ok := toolsMap[fmt.Sprintf("%s-%s-%s", tm.Release, tm.Version, tm.Arch)]; !ok {
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

func WriteMetadata(toolsList coretools.List, fetch bool, metadataStore storage.Storage) error {
	// Read any existing metadata so we can merge the new tools metadata with what's there.
	// The metadata from toolsList is already present, the existing data is overwritten.
	dataSource := storage.NewStorageSimpleStreamsDataSource(metadataStore, "tools")
	toolsConstraint, err := makeToolsConstraint(simplestreams.CloudSpec{}, -1, -1, coretools.Filter{})
	if err != nil {
		return err
	}
	existingMetadata, err := Fetch([]simplestreams.DataSource{dataSource}, simplestreams.DefaultIndexPath, toolsConstraint, false)
	if err != nil && !errors.IsNotFoundError(err) {
		return err
	}
	newToolsVersions := make(map[string]bool)
	for _, tool := range toolsList {
		newToolsVersions[tool.Version.String()] = true
	}
	// Merge in existing records.
	for _, toolsMetadata := range existingMetadata {
		vers := version.Binary{version.MustParse(toolsMetadata.Version), toolsMetadata.Release, toolsMetadata.Arch}
		if _, ok := newToolsVersions[vers.String()]; ok {
			continue
		}
		tool := &coretools.Tools{
			Version: vers,
			SHA256:  toolsMetadata.SHA256,
			Size:    toolsMetadata.Size,
		}
		toolsList = append(toolsList, tool)
	}
	metadataInfo, err := generateMetadata(toolsList, fetch)
	if err != nil {
		return err
	}
	for _, md := range metadataInfo {
		logger.Infof("Writing %s", "tools/"+md.Path)
		err = metadataStore.Put("tools/"+md.Path, bytes.NewReader(md.Data), int64(len(md.Data)))
		if err != nil {
			return err
		}
	}
	return nil
}

func generateMetadata(toolsList coretools.List, fetch bool) ([]MetadataFile, error) {
	metadata := make([]*ToolsMetadata, len(toolsList))
	for i, t := range toolsList {
		var size int64
		var sha256hex string
		var err error
		if fetch && t.Size == 0 {
			logger.Infof("Fetching tools to generate hash: %v", t.URL)
			var sha256hash hash.Hash
			size, sha256hash, err = fetchToolsHash(t.URL)
			if err != nil {
				return nil, err
			}
			sha256hex = fmt.Sprintf("%x", sha256hash.Sum(nil))
		} else {
			size = t.Size
			sha256hex = t.SHA256
		}

		path := fmt.Sprintf("releases/juju-%s-%s-%s.tgz", t.Version.Number, t.Version.Series, t.Version.Arch)
		metadata[i] = &ToolsMetadata{
			Release:  t.Version.Series,
			Version:  t.Version.Number.String(),
			Arch:     t.Version.Arch,
			Path:     path,
			FileType: "tar.gz",
			Size:     size,
			SHA256:   sha256hex,
		}
	}

	index, products, err := MarshalToolsMetadataJSON(metadata, time.Now())
	if err != nil {
		return nil, err
	}
	objects := []MetadataFile{
		{simplestreams.UnsignedIndex, index},
		{ProductMetadataPath, products},
	}
	return objects, nil
}

// fetchToolsHash fetches the file at the specified URL,
// and calculates its size in bytes and computes a SHA256
// hash of its contents.
func fetchToolsHash(url string) (size int64, sha256hash hash.Hash, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, nil, err
	}
	sha256hash = sha256.New()
	size, err = io.Copy(sha256hash, resp.Body)
	resp.Body.Close()
	return size, sha256hash, err
}
