// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/core/arch"
)

const (
	// SeriesAll defines a platform that targets all series.
	SeriesAll = "all"
	// ArchAll defines a platform that targets all architectures.
	ArchAll = "all"
)

func convertCharmInfoResult(info charmhub.InfoResponse, arch, series string) (InfoResponse, error) {
	channels := filterChannels(info.Channels, arch, series)

	ir := InfoResponse{
		Type:        info.Type,
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Publisher:   info.Publisher,
		Summary:     info.Summary,
		Series:      info.Series,
		StoreURL:    info.StoreURL,
		Tags:        info.Tags,
		Channels:    convertChannels(channels),
		Tracks:      info.Tracks,
	}

	var err error
	switch ir.Type {
	case "bundle":
		ir.Bundle, err = convertBundle(info.Bundle)
	case "charm":
		ir.Charm, err = convertCharm(info.Charm)
	}
	return ir, errors.Trace(err)
}

func convertCharmFindResults(responses []charmhub.FindResponse) []FindResponse {
	results := make([]FindResponse, len(responses))
	for i, resp := range responses {
		results[i] = convertCharmFindResult(resp)
	}
	return results
}

func convertCharmFindResult(resp charmhub.FindResponse) FindResponse {
	return FindResponse{
		Type:      resp.Type,
		ID:        resp.ID,
		Name:      resp.Name,
		Publisher: resp.Publisher,
		Summary:   resp.Summary,
		Version:   resp.Version,
		Arches:    resp.Arches,
		Series:    resp.Series,
		StoreURL:  resp.StoreURL,
	}
}

func convertBundle(in interface{}) (*Bundle, error) {
	inB, ok := in.(*charmhub.Bundle)
	if !ok {
		return nil, errors.Errorf("unexpected: value is not a bundle")
	}
	if inB == nil {
		return nil, errors.Errorf("bundle is nil")
	}
	out := Bundle{
		Charms: make([]BundleCharm, len(inB.Charms)),
	}
	for i, c := range inB.Charms {
		out.Charms[i] = BundleCharm{Name: c.Name}
	}
	return &out, nil
}

func convertCharm(in interface{}) (*Charm, error) {
	inC, ok := in.(*charmhub.Charm)
	if !ok {
		return nil, errors.Errorf("unexpected: value is not a charm")
	}
	if inC == nil {
		return nil, errors.Errorf("CharmHubCharm is nil")
	}
	return &Charm{
		Config:      inC.Config,
		Relations:   inC.Relations,
		Subordinate: inC.Subordinate,
		UsedBy:      inC.UsedBy,
	}, nil
}

func convertChannels(in map[string]charmhub.Channel) map[string]Channel {
	out := make(map[string]Channel, len(in))
	for k, v := range in {
		out[k] = Channel{
			ReleasedAt: v.ReleasedAt,
			Track:      v.Track,
			Risk:       v.Risk,
			Revision:   v.Revision,
			Size:       v.Size,
			Version:    v.Version,
			Series:     channelSeries(v.Platforms).SortedValues(),
			Arches:     channelArches(v.Platforms).SortedValues(),
		}
	}

	return out
}

func filterChannels(in map[string]charmhub.Channel, architecture, series string) map[string]charmhub.Channel {
	allArch := architecture == ArchAll
	allSeries := series == SeriesAll

	// If we're searching for everything then we can skip the filtering logic
	// and return immediately.
	if allArch && allSeries {
		return in
	}

	// Channels that match any part of the criteria should be witnessed and
	// kept.
	witnessed := make(map[string]charmhub.Channel)

	for k, v := range in {
		archSet := channelArches(v.Platforms)
		seriesSet := channelSeries(v.Platforms)

		if (allArch || archSet.Contains(architecture)) &&
			(allSeries || seriesSet.Contains(series) || seriesSet.Contains(SeriesAll)) {
			witnessed[k] = v
		}
	}
	return witnessed
}

func channelSeries(platforms []charmhub.Platform) set.Strings {
	series := set.NewStrings()
	for _, v := range platforms {
		series.Add(v.Series)
	}
	return series
}

func channelArches(platforms []charmhub.Platform) set.Strings {
	arches := set.NewStrings()
	for _, v := range platforms {
		arches.Add(v.Architecture)
	}
	// If the platform contains all the arches, just return them exploded.
	// This makes the filtering logic simpler for plucking an architecture out
	// of the channels, we should aim to do the same for series.
	if arches.Contains(ArchAll) {
		return set.NewStrings(arch.AllArches().StringList()...)
	}
	return arches
}

func filterFindResults(in []charmhub.FindResponse, architecture, series string) []charmhub.FindResponse {
	allArch := architecture == ArchAll
	allSeries := series == SeriesAll

	// If we're searching for everything then we can skip the filtering logic
	// and return immediately.
	if allArch && allSeries {
		return in
	}

	witnessed := make([]charmhub.FindResponse, 0)

	for _, resp := range in {
		archSet := set.NewStrings(resp.Arches...)
		if archSet.Contains(ArchAll) {
			archSet = set.NewStrings(arch.AllArches().StringList()...)
		}
		seriesSet := set.NewStrings(resp.Series...)

		if (allArch || archSet.Contains(architecture)) &&
			(allSeries || seriesSet.Contains(series) || seriesSet.Contains(SeriesAll)) {
			witnessed = append(witnessed, resp)
		}
	}

	return witnessed
}

type InfoResponse struct {
	Type        string             `json:"type" yaml:"type"`
	ID          string             `json:"id" yaml:"id"`
	Name        string             `json:"name" yaml:"name"`
	Description string             `json:"description" yaml:"description"`
	Publisher   string             `json:"publisher" yaml:"publisher"`
	Summary     string             `json:"summary" yaml:"summary"`
	Series      []string           `json:"series,omitempty" yaml:"series,omitempty"`
	StoreURL    string             `json:"store-url" yaml:"store-url"`
	Tags        []string           `json:"tags,omitempty" yaml:"tags,omitempty"`
	Charm       *Charm             `json:"charm,omitempty" yaml:"charm,omitempty"`
	Bundle      *Bundle            `json:"bundle,omitempty" yaml:"bundle,omitempty"`
	Channels    map[string]Channel `json:"channel-map" yaml:"channel-map"`
	Tracks      []string           `json:"tracks,omitempty" yaml:"tracks,omitempty"`
}

type FindResponse struct {
	Type      string   `json:"type" yaml:"type"`
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Publisher string   `json:"publisher" yaml:"publisher"`
	Summary   string   `json:"summary" yaml:"summary"`
	Version   string   `json:"version" yaml:"version"`
	Arches    []string `json:"architectures,omitempty" yaml:"architectures,omitempty"`
	Series    []string `json:"series,omitempty" yaml:"series,omitempty"`
	StoreURL  string   `json:"store-url" yaml:"store-url"`
}

type Channel struct {
	ReleasedAt string   `json:"released-at" yaml:"released-at"`
	Track      string   `json:"track" yaml:"track"`
	Risk       string   `json:"risk" yaml:"risk"`
	Revision   int      `json:"revision" yaml:"revision"`
	Size       int      `json:"size" yaml:"size"`
	Version    string   `json:"version" yaml:"version"`
	Arches     []string `json:"architectures" yaml:"architectures"`
	Series     []string `json:"series" yaml:"series"`
}

// Charm matches a params.CharmHubCharm
type Charm struct {
	Config      *charm.Config                `json:"config,omitempty" yaml:"config,omitempty"`
	Relations   map[string]map[string]string `json:"relations,omitempty" yaml:"relations,omitempty"`
	Subordinate bool                         `json:"subordinate,omitempty" yaml:"subordinate,omitempty"`
	UsedBy      []string                     `json:"used-by,omitempty" yaml:"used-by,omitempty"` // bundles which use the charm
}

type Bundle struct {
	Charms []BundleCharm `json:"charms,omitempty" yaml:"charms,omitempty"`
}

type BundleCharm struct {
	Name string `json:"name" yaml:"name"`
}
