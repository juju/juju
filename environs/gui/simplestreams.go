// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/version"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju"
	jujuversion "github.com/juju/juju/version"
)

const (
	// DefaultBaseURL holds the default simplestreams data source URL from
	// where to retrieve Juju GUI archives.
	DefaultBaseURL = "https://streams.canonical.com/juju/gui"
	// ReleasedStream and DevelStreams hold stream names to use when fetching
	// Juju GUI archives.
	ReleasedStream = "released"
	DevelStream    = "devel"

	downloadType      = "content-download"
	sourceDescription = "gui simplestreams"
	streamsVersion    = "v1"
)

func init() {
	simplestreams.RegisterStructTags(Metadata{})
}

// DataSource creates and returns a new simplestreams signed data source for
// fetching Juju GUI archives, at the given URL.
func NewDataSource(baseURL string) simplestreams.DataSource {
	requireSigned := true
	return simplestreams.NewURLSignedDataSource(
		sourceDescription,
		baseURL,
		juju.JujuPublicKey,
		utils.VerifySSLHostnames,
		simplestreams.DEFAULT_CLOUD_DATA,
		requireSigned)
}

// FetchMetadata fetches and returns Juju GUI metadata from simplestreams,
// sorted by version descending.
func FetchMetadata(stream string, sources ...simplestreams.DataSource) ([]*Metadata, error) {
	params := simplestreams.GetMetadataParams{
		StreamsVersion: streamsVersion,
		LookupConstraint: &constraint{
			LookupParams: simplestreams.LookupParams{Stream: stream},
			majorVersion: jujuversion.Current.Major,
		},
		ValueParams: simplestreams.ValueParams{
			DataType:        downloadType,
			MirrorContentId: contentId(stream),
			FilterFunc:      appendArchives,
			ValueTemplate:   Metadata{},
		},
	}
	items, _, err := simplestreams.GetMetadata(sources, params)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching simplestreams metadata")
	}
	allMeta := make([]*Metadata, len(items))
	for i, item := range items {
		allMeta[i] = item.(*Metadata)
	}
	sort.Sort(byVersion(allMeta))
	return allMeta, nil
}

// Metadata is the type used to retrieve GUI archive metadata information from
// simplestream. Tags for this structure are registered in init().
type Metadata struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`

	JujuMajorVersion int    `json:"juju-version"`
	StringVersion    string `json:"version"`

	Version  version.Number           `json:"-"`
	FullPath string                   `json:"-"`
	Source   simplestreams.DataSource `json:"-"`
}

// byVersion is used to sort GUI metadata by version, most recent first.
type byVersion []*Metadata

// Len implements sort.Interface.
func (b byVersion) Len() int { return len(b) }

// Swap implements sort.Interface.
func (b byVersion) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

// Less implements sort.Interface.
func (b byVersion) Less(i, j int) bool { return b[i].Version.Compare(b[j].Version) > 0 }

// constraint is used as simplestreams.LookupConstraint when retrieving Juju
// GUI metadata information.
type constraint struct {
	simplestreams.LookupParams
	majorVersion int
}

// IndexIds generates a string array representing index ids formed similarly to
// an ISCSI qualified name (IQN).
func (c *constraint) IndexIds() []string {
	return []string{contentId(c.Stream)}
}

// ProductIds generates a string array representing product ids formed
// similarly to an ISCSI qualified name (IQN).
func (c *constraint) ProductIds() ([]string, error) {
	return []string{"com.canonical.streams:gui"}, nil
}

// contentId returns the GUI content id in simplestreams for the given stream.
func contentId(stream string) string {
	return fmt.Sprintf("com.canonical.streams:%s:gui", stream)
}

// appendArchives collects all matching Juju GUI archive metadata information.
func appendArchives(
	source simplestreams.DataSource,
	matchingItems []interface{},
	items map[string]interface{},
	cons simplestreams.LookupConstraint,
) ([]interface{}, error) {
	var majorVersion int
	if guiConstraint, ok := cons.(*constraint); ok {
		majorVersion = guiConstraint.majorVersion
	}
	for _, item := range items {
		meta := item.(*Metadata)
		if majorVersion != 0 && majorVersion != meta.JujuMajorVersion {
			continue
		}
		fullPath, err := source.URL(meta.Path)
		if err != nil {
			return nil, errors.Annotate(err, "cannot retrieve metadata full path")
		}
		meta.FullPath = fullPath
		vers, err := version.Parse(meta.StringVersion)
		if err != nil {
			return nil, errors.Annotate(err, "cannot parse metadata version")
		}
		meta.Version = vers
		meta.Source = source
		matchingItems = append(matchingItems, meta)
	}
	return matchingItems, nil
}
