// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/charm/v7"
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

func convertCharmFindResult(info params.FindResponse) FindResponse {
	return FindResponse{
		Type: info.Type,
		ID:   info.ID,
		Name: info.Name,
		//Entity:         convertEntity(info.Entity),
		//DefaultRelease: convertOneChannelMap(info.DefaultRelease),
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
		Tags:        inC.Tags,
		UsedBy:      inC.UsedBy,
	}
}

func convertChannels(in map[string]params.Channel) map[string]Channel {
	out := make(map[string]Channel, len(in))
	for k, v := range in {
		out[k] = Channel(v)
	}
	return out
}

// Although InfoResponse or FindResponse are similar, they will change once the
// charmhub API has settled.

type InfoResponse struct {
	Type        string             `json:"type"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Publisher   string             `json:"publisher"`
	Summary     string             `json:"summary"`
	Series      []string           `json:"series"`
	StoreURL    string             `json:"store-url"`
	Charm       *Charm             `json:"charm,omitempty"`
	Bundle      *Bundle            `json:"bundle,omitempty"`
	Channels    map[string]Channel `json:"channel-map"`
	Tracks      []string           `json:"tracks"`
}

type FindResponse struct {
	Type           string     `json:"type"`
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Entity         Entity     `json:"entity"`
	DefaultRelease ChannelMap `json:"default-release,omitempty"`
}

type ChannelMap struct {
	Channel Channel `json:"channel,omitempty"`
	//Revision Revision `json:"revision,omitempty"`
}

type Channel struct {
	ReleasedAt string `json:"released-at"`
	Track      string `json:"track"`
	Risk       string `json:"risk"`
	Revision   int    `json:"revision"`
	Size       int    `json:"size"`
	Version    string `json:"version"`
}

type Entity struct {
	//Categories  []Category        `json:"categories"`
	Description string `json:"description"`
	License     string `json:"license"`
	//Media       []Media           `json:"media"`
	Publisher map[string]string `json:"publisher"`
	Summary   string            `json:"summary"`
	UsedBy    []string          `json:"used-by"` // bundles which use the charm
}

// Charm matches a params.CharmHubCharm
type Charm struct {
	Config      *charm.Config                `json:"config"`
	Relations   map[string]map[string]string `json:"relations"`
	Subordinate bool                         `json:"subordinate"`
	Tags        []string                     `json:"tags"`
	UsedBy      []string                     `json:"used-by"` // bundles which use the charm
}

type Bundle struct {
	Charms []BundleCharm `json:"charms,"`
}

type BundleCharm struct {
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}
