// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package selector

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/charm"
)

const (
	// SeriesAll defines a platform that targets all series.
	SeriesAll = "all"
	// ArchAll defines a platform that targets all architectures.
	ArchAll = "all"
)

// Selector defines a way of selecting a resource URL from a info response.
type Selector struct {
	orderedSeries []string
	emptyArch     string
	emptySeries   string
}

// NewSelectorForBundle creates a selector that uses empty arch and series to
// locate values in the info response.
func NewSelectorForBundle(orderedSeries []string) *Selector {
	return &Selector{
		orderedSeries: orderedSeries,
		emptyArch:     "",
		emptySeries:   "",
	}
}

// NewSelectorForDownload creates a selector that uses empty arch and series to
// locate values in the info response.
func NewSelectorForDownload(orderedSeries []string) *Selector {
	return &Selector{
		orderedSeries: orderedSeries,
		emptyArch:     ArchAll,
		emptySeries:   SeriesAll,
	}
}

// Locate will atttempt to locate a resource url using a charm origin.
func (c *Selector) Locate(info transport.InfoResponse, origin charm.Origin) (*url.URL, charm.Origin, error) {
	var (
		filterFn FilterInfoChannelMapFunc

		channel = origin.Channel
		arch    = origin.Platform.Architecture
		series  = origin.Platform.Series

		selectAll = arch == c.emptyArch && series == c.emptySeries
	)
	switch {
	case selectAll:
		// Select every channel for all architectures and all series.
		filterFn = func(transport.InfoChannelMap) bool {
			return true
		}
	case arch != c.emptyArch && series == c.emptySeries:
		filterFn = c.filterByArchitecture(arch)
	case arch == c.emptyArch && series != c.emptySeries:
		filterFn = c.filterBySeries(series)
	default:
		filterFn = c.filterByArchitectureAndSeries(arch, series)
	}

	if channel == nil {
		defaultRelease, err := charm.MakeChannel("latest", "stable", "")
		if err != nil {
			return nil, charm.Origin{}, errors.Trace(err)
		}
		channel = &defaultRelease
	}

	// To allow users to download from closed channels (same UX for snaps), we
	// link all closed channels to their parent channel, if their parent channel
	// is also closed, we move to the next.
	// We do the linking here because we want to ensure that we only ever filter
	// once ensuring we're always correct.
	channelMap, err := linkClosedChannels(info.ChannelMap)
	if err != nil {
		return nil, charm.Origin{}, errors.Trace(err)
	}

	channelMap = filterInfoChannelMap(channelMap, filterFn)
	revisions, found := locateRevisionByChannel(c.sortInfoChannelMap(channelMap), *channel)
	if !found {
		if series != "" {
			return nil, charm.Origin{}, errors.Errorf("%s %q not found for %q channel matching %q series", info.Type, info.Name, channel, series)
		}
		return nil, charm.Origin{}, errors.Errorf("%s %q not found within the channel %q", info.Type, info.Name, channel)
	}

	if selectAll {
		// Ensure we have just one architecture.
		if arches := listArchitectures(revisions); len(arches) > 1 {
			list := strings.Join(arches, ",")
			return nil, charm.Origin{}, errors.Errorf("multiple architectures (%s) found, use --arch flag to specify the one to download", list)
		}
	}
	if len(revisions) == 0 {
		return nil, charm.Origin{}, errors.Errorf("%s %q not found within the channel %q", info.Type, info.Name, channel)
	}

	revision := revisions[0]
	resourceURL, err := url.Parse(revision.Download.URL)
	if err != nil {
		return nil, charm.Origin{}, errors.Trace(err)
	}

	resultOrigin := origin
	resultOrigin.Type = info.Type
	resultOrigin.Revision = &revision.Revision

	return resourceURL, resultOrigin, nil
}

func (c *Selector) sortInfoChannelMap(in []transport.InfoChannelMap) []transport.InfoChannelMap {
	// Order the channelMap by the ordered supported controller series. That
	// way we'll always find the newest one first (hopefully the most
	// supported).
	// Then attempt to find the revision by a channel.
	channelMap := channelMapBySeries{
		channelMap: in,
		series:     c.orderedSeries,
	}
	sort.Sort(channelMap)

	return channelMap.channelMap
}

func (c *Selector) filterByArchitecture(arch string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		if arch == c.emptyArch {
			return true
		}

		platformArch := channelMap.Channel.Platform.Architecture
		return (platformArch == arch || platformArch == c.emptyArch) ||
			isArchInPlatforms(channelMap.Revision.Platforms, arch) ||
			isArchInPlatforms(channelMap.Revision.Platforms, ArchAll)
	}
}

func (c *Selector) filterBySeries(series string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		if series == c.emptySeries {
			return true
		}

		platformSeries := channelMap.Channel.Platform.Series
		return (platformSeries == series || platformSeries == c.emptySeries) ||
			isSeriesInPlatforms(channelMap.Revision.Platforms, series) ||
			isSeriesInPlatforms(channelMap.Revision.Platforms, SeriesAll)
	}
}

func (c *Selector) filterByArchitectureAndSeries(arch, series string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		return c.filterByArchitecture(arch)(channelMap) &&
			c.filterBySeries(series)(channelMap)
	}
}

// FilterInfoChannelMapFunc is a type alias for representing a filter function.
type FilterInfoChannelMapFunc func(channelMap transport.InfoChannelMap) bool

func filterInfoChannelMap(in []transport.InfoChannelMap, fn FilterInfoChannelMapFunc) []transport.InfoChannelMap {
	var filtered []transport.InfoChannelMap
	for _, channelMap := range in {
		if !fn(channelMap) {
			continue
		}
		filtered = append(filtered, channelMap)
	}
	return filtered
}

func locateRevisionByChannel(channelMaps []transport.InfoChannelMap, channel charm.Channel) ([]transport.InfoRevision, bool) {
	var (
		revisions []transport.InfoRevision
		found     bool
	)
	channel = channel.Normalize()
	for _, channelMap := range channelMaps {
		if rev, ok := locateRevisionByChannelMap(channelMap, channel); ok {
			revisions = append(revisions, rev)
			found = true
		}
	}
	return revisions, found
}

func listArchitectures(revisions []transport.InfoRevision) []string {
	arches := set.NewStrings()
	for _, rev := range revisions {
		for _, platform := range rev.Platforms {
			arches.Add(platform.Architecture)
		}
	}
	return arches.SortedValues()
}

type channelMapIndex struct {
	witnessed  bool
	channelMap transport.InfoChannelMap
}

var channelRisks = []string{"stable", "candidate", "beta", "edge"}

func linkClosedChannels(channelMaps []transport.InfoChannelMap) ([]transport.InfoChannelMap, error) {
	witness := make(map[string][]channelMapIndex)
	for _, channelMap := range channelMaps {
		track := channelMap.Channel.Track
		if witness[track] == nil {
			witness[track] = make([]channelMapIndex, len(channelRisks))
		}
		index := riskIndex(channelMap)
		if index < 0 {
			return nil, errors.Errorf("invalid risk %q", channelMap.Channel.Risk)
		}
		witness[track][index] = channelMapIndex{
			witnessed:  true,
			channelMap: channelMap,
		}
	}

	secondary := make(map[string][]transport.InfoChannelMap)
	for track, channels := range witness {
		secondary[track] = make([]transport.InfoChannelMap, len(channelRisks))

		for i, channel := range channels {
			if channel.witnessed {
				secondary[track][i] = channel.channelMap
				continue
			}

			k := i
			for ; k >= 0; k-- {
				if channels[k].witnessed {
					break
				}
			}

			if k == -1 {
				continue
			}

			link := channels[k].channelMap
			link.Channel.Risk = channelRisks[i]
			secondary[track][i] = link
		}
	}

	var result []transport.InfoChannelMap
	for _, risks := range secondary {
		for _, channel := range risks {
			result = append(result, channel)
		}
	}
	return result, nil
}

func riskIndex(ch transport.InfoChannelMap) int {
	for i, risk := range channelRisks {
		if risk == ch.Channel.Risk {
			return i
		}
	}
	return -1
}

func isSeriesInPlatforms(platforms []transport.Platform, series string) bool {
	for _, platform := range platforms {
		if platform.Series == series {
			return true
		}
	}
	return false
}

func isArchInPlatforms(platforms []transport.Platform, arch string) bool {
	for _, platform := range platforms {
		if platform.Architecture == arch {
			return true
		}
	}
	return false
}

func constructChannelFromTrackAndRisk(track, risk string) (charm.Channel, error) {
	rawChannel := fmt.Sprintf("%s/%s", track, risk)
	if strings.HasPrefix(rawChannel, "/") {
		rawChannel = rawChannel[1:]
	} else if strings.HasSuffix(rawChannel, "/") {
		rawChannel = rawChannel[:len(rawChannel)-1]
	}
	return charm.ParseChannelNormalize(rawChannel)
}

func locateRevisionByChannelMap(channelMap transport.InfoChannelMap, channel charm.Channel) (transport.InfoRevision, bool) {
	charmChannel, err := constructChannelFromTrackAndRisk(channelMap.Channel.Track, channelMap.Channel.Risk)
	if err != nil {
		return transport.InfoRevision{}, false
	}

	// Check that we're an exact match.
	if channel.Track == charmChannel.Track && channel.Risk == charmChannel.Risk {
		return channelMap.Revision, true
	}

	return transport.InfoRevision{}, false
}

type channelMapBySeries struct {
	channelMap []transport.InfoChannelMap
	series     []string
}

func (s channelMapBySeries) Len() int {
	return len(s.channelMap)
}

func (s channelMapBySeries) Swap(i, j int) {
	s.channelMap[i], s.channelMap[j] = s.channelMap[j], s.channelMap[i]
}

func (s channelMapBySeries) Less(i, j int) bool {
	idx1 := s.invertedIndexOf(s.channelMap[i].Channel.Platform.Series)
	idx2 := s.invertedIndexOf(s.channelMap[j].Channel.Platform.Series)
	return idx1 > idx2
}

func (s channelMapBySeries) invertedIndexOf(value string) int {
	for k, i := range s.series {
		if i == value {
			return len(s.series) - k
		}
	}
	return math.MinInt64
}
