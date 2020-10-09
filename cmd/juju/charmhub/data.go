// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/api/charmhub"
)

func convertCharmInfoResult(info charmhub.InfoResponse, series string) (InfoResponse, error) {
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
		Channels:    convertChannels(info.Channels, series),
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

func convertChannels(in map[string]charmhub.Channel, series string) map[string]Channel {
	out := make(map[string]Channel, len(in))
	for k, v := range in {
		if series != "" {
			if !channelSeries(v.Platforms).Contains(series) {
				break
			}
		}
		out[k] = Channel{
			ReleasedAt: v.ReleasedAt,
			Track:      v.Track,
			Risk:       v.Risk,
			Revision:   v.Revision,
			Size:       v.Size,
			Version:    v.Version,
		}
	}
	return out
}

func channelSeries(platforms []charmhub.Platform) set.Strings {
	series := set.NewStrings()
	for _, v := range platforms {
		series.Add(v.Series)
	}
	return series
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
	Series    []string `json:"series,omitempty" yaml:"series,omitempty"`
	StoreURL  string   `json:"store-url" yaml:"store-url"`
}

type Channel struct {
	ReleasedAt string `json:"released-at" yaml:"released-at"`
	Track      string `json:"track" yaml:"track"`
	Risk       string `json:"risk" yaml:"risk"`
	Revision   int    `json:"revision" yaml:"revision"`
	Size       int    `json:"size" yaml:"size"`
	Version    string `json:"version" yaml:"version"`
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
