// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/keys"
)

const (
	// DefaultBaseURL holds the default simplestreams data source URL from
	// where to retrieve Juju Dashboard archives.
	DefaultBaseURL = "https://streams.canonical.com/juju/gui"
	// ReleasedStream and DevelStreams hold stream names to use when fetching
	// Juju Dashboard archives.
	ReleasedStream = "released"
	DevelStream    = "devel"

	downloadType      = "content-download"
	sourceDescription = "dashboard simplestreams"
	streamsVersion    = "v1"
)

func init() {
	simplestreams.RegisterStructTags(Metadata{})
}

// DataSource creates and returns a new simplestreams signed data source for
// fetching Juju Dashboard archives, at the given URL.
func NewDataSource(baseURL string) simplestreams.DataSource {
	return simplestreams.NewDataSource(
		simplestreams.Config{
			Description:          sourceDescription,
			BaseURL:              baseURL,
			PublicSigningKey:     keys.JujuPublicKey,
			HostnameVerification: utils.VerifySSLHostnames,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
			RequireSigned:        true,
		},
	)
}

// FetchMetadata fetches and returns Juju Dashboard metadata from simplestreams,
// sorted by version descending.
func FetchMetadata(stream string, major, minor int, sources ...simplestreams.DataSource) ([]*Metadata, error) {
	params := simplestreams.GetMetadataParams{
		StreamsVersion: streamsVersion,
		LookupConstraint: &constraint{
			LookupParams: simplestreams.LookupParams{Stream: stream},
			majorVersion: major,
			minorVersion: minor,
		},
		ValueParams: simplestreams.ValueParams{
			DataType:        downloadType,
			MirrorContentId: contentId(stream),
			FilterFunc:      appendArchives,
			ValueTemplate:   Metadata{},
		},
	}
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	items, _, err := ss.GetMetadata(sources, params)
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

// Metadata is the type used to retrieve Dashboard archive metadata information from
// simplestream. Tags for this structure are registered in init().
type Metadata struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`

	MinJujuVersion   string `json:"min-juju-version"`
	DashboardVersion string `json:"version"`

	Version  version.Number           `json:"-"`
	FullPath string                   `json:"-"`
	Source   simplestreams.DataSource `json:"-"`
}

// byVersion is used to sort Dashboard metadata by version, most recent first.
type byVersion []*Metadata

// Len implements sort.Interface.
func (b byVersion) Len() int { return len(b) }

// Swap implements sort.Interface.
func (b byVersion) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

// Less implements sort.Interface.
func (b byVersion) Less(i, j int) bool { return b[i].Version.Compare(b[j].Version) > 0 }

// constraint is used as simplestreams.LookupConstraint when retrieving Juju
// Dashboard metadata information.
type constraint struct {
	simplestreams.LookupParams
	majorVersion int
	minorVersion int
}

// IndexIds generates a string array representing index ids formed similarly to
// an ISCSI qualified name (IQN).
func (c *constraint) IndexIds() []string {
	return []string{contentId(c.Stream)}
}

// ProductIds generates a string array representing product ids formed
// similarly to an ISCSI qualified name (IQN).
func (c *constraint) ProductIds() ([]string, error) {
	return []string{"com.canonical.streams:dashboard"}, nil
}

// contentId returns the dashboard content id in simplestreams for the given stream.
func contentId(stream string) string {
	return fmt.Sprintf("com.canonical.streams:%s:dashboard", stream)
}

// majorMinorRegEx is used to validate a major.minor version string.
var majorMinorRegEx = regexp.MustCompile("^\\d\\.\\d$")

// appendArchives collects all matching Juju Dashboard archive metadata information.
func appendArchives(
	source simplestreams.DataSource,
	matchingItems []interface{},
	items map[string]interface{},
	cons simplestreams.LookupConstraint,
) ([]interface{}, error) {
	var majorVersion int
	var minorVersion int
	if dashboardConstraint, ok := cons.(*constraint); ok {
		majorVersion = dashboardConstraint.majorVersion
		minorVersion = dashboardConstraint.minorVersion
	}
	for _, item := range items {
		meta := item.(*Metadata)
		if meta.MinJujuVersion != "" && !majorMinorRegEx.MatchString(meta.MinJujuVersion) {
			return nil, errors.NotValidf("min-juju-version value %q", meta.MinJujuVersion)
		}
		if meta.MinJujuVersion != "" {
			// Add a ".0" to major.minor to make a valid Juju version number.
			minJujuVersion, err := version.Parse(meta.MinJujuVersion + ".0")
			if err != nil {
				return nil, errors.Annotate(err, "cannot parse supported juju version")
			}
			if majorVersion != minJujuVersion.Major ||
				minorVersion < minJujuVersion.Minor {
				continue
			}
		}
		fullPath, err := source.URL(meta.Path)
		if err != nil {
			return nil, errors.Annotate(err, "cannot retrieve metadata full path")
		}
		meta.FullPath = fullPath
		vers, err := version.Parse(meta.DashboardVersion)
		if err != nil {
			return nil, errors.Annotate(err, "cannot parse metadata version")
		}
		meta.Version = vers
		meta.Source = source
		matchingItems = append(matchingItems, meta)
	}
	return matchingItems, nil
}
