// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/charm/v8"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.charmhub")

func convertCharmInfoResult(info params.InfoResponse) InfoResponse {
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
		Channels:    convertChannels(info.Channels),
		Tracks:      info.Tracks,
	}
	switch ir.Type {
	case "bundle":
		ir.Bundle = convertBundle(info.Bundle)
	case "charm":
		ir.Charm = convertCharm(info.Charm)
	}
	return ir
}

func convertCharmFindResults(responses []params.FindResponse) []FindResponse {
	results := make([]FindResponse, len(responses))
	for i, resp := range responses {
		results[i] = convertCharmFindResult(resp)
	}
	return results
}

func convertCharmFindResult(resp params.FindResponse) FindResponse {
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

func convertBundle(in interface{}) *Bundle {
	inB, ok := in.(*params.CharmHubBundle)
	if !ok {
		logger.Errorf("unexpected: CharmHubBundle is not a bundle")
		return nil
	}
	if inB == nil {
		logger.Errorf("CharmHubBundle is nil")
		return nil
	}
	out := Bundle{
		Charms: make([]BundleCharm, len(inB.Charms)),
	}
	for i, c := range inB.Charms {
		out.Charms[i] = BundleCharm(c)
	}
	return &out
}

func convertCharm(in interface{}) *Charm {
	inC, ok := in.(*params.CharmHubCharm)
	if !ok {
		logger.Errorf("unexpected: CharmHubCharm is not a charm")
		return nil
	}
	if inC == nil {
		logger.Errorf("CharmHubCharm is nil")
		return nil
	}
	return &Charm{
		Config:      params.FromCharmOptionMap(inC.Config),
		Relations:   inC.Relations,
		Subordinate: inC.Subordinate,
		UsedBy:      inC.UsedBy,
	}
}

func convertChannels(in map[string]params.Channel) map[string]Channel {
	out := make(map[string]Channel, len(in))
	for k, v := range in {
		out[k] = Channel{
			ReleasedAt: v.ReleasedAt,
			Track:      v.Track,
			Risk:       v.Risk,
			Revision:   v.Revision,
			Size:       v.Size,
			Version:    v.Version,
			Platforms:  convertPlatforms(v.Platforms),
		}
	}
	return out
}

func convertPlatforms(in []params.Platform) []Platform {
	out := make([]Platform, len(in))
	for i, v := range in {
		out[i] = Platform(v)
	}
	return out
}

// Although InfoResponse or FindResponse are similar, they will change once the
// CharmHub API has settled.

type InfoResponse struct {
	Type        string             `json:"type"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Publisher   string             `json:"publisher"`
	Summary     string             `json:"summary"`
	Series      []string           `json:"series,omitempty"`
	StoreURL    string             `json:"store-url"`
	Tags        []string           `json:"tags,omitempty"`
	Charm       *Charm             `json:"charm,omitempty"`
	Bundle      *Bundle            `json:"bundle,omitempty"`
	Channels    map[string]Channel `json:"channel-map"`
	Tracks      []string           `json:"tracks"`
}

type FindResponse struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Publisher string   `json:"publisher"`
	Summary   string   `json:"summary"`
	Version   string   `json:"version"`
	Series    []string `json:"series,omitempty"`
	StoreURL  string   `json:"store-url"`
}

type Channel struct {
	ReleasedAt string     `json:"released-at"`
	Track      string     `json:"track"`
	Risk       string     `json:"risk"`
	Revision   int        `json:"revision"`
	Size       int        `json:"size"`
	Version    string     `json:"version"`
	Platforms  []Platform `json:"platforms"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Series       string `json:"series"`
}

// Charm matches a params.CharmHubCharm
type Charm struct {
	Config      *charm.Config                `json:"config,omitempty"`
	Relations   map[string]map[string]string `json:"relations,omitempty"`
	Subordinate bool                         `json:"subordinate,omitempty"`
	UsedBy      []string                     `json:"used-by,omitempty"` // bundles which use the charm
}

type Bundle struct {
	Charms []BundleCharm `json:"charms,omitempty"`
}

type BundleCharm struct {
	Name      string `json:"name"`
	PackageID string `json:"package-id"`
}
