// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/juju/apiserver/params"
)

func convertCharmInfoResult(info params.InfoResponse) InfoResponse {
	return InfoResponse{
		Type:           info.Type,
		ID:             info.ID,
		Name:           info.Name,
		Charm:          convertCharm(info.Charm),
		ChannelMap:     convertChannelMap(info.ChannelMap),
		DefaultRelease: convertOneChannelMap(info.DefaultRelease),
	}
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
		Type:           info.Type,
		ID:             info.ID,
		Name:           info.Name,
		Charm:          convertCharm(info.Charm),
		ChannelMap:     convertChannelMap(info.ChannelMap),
		DefaultRelease: convertOneChannelMap(info.DefaultRelease),
	}
}

func convertCharm(ch params.CharmHubCharm) Charm {
	return Charm{
		Categories:  convertCategories(ch.Categories),
		Description: ch.Description,
		License:     ch.License,
		Media:       convertMedia(ch.Media),
		Publisher:   ch.Publisher,
		Summary:     ch.Summary,
		UsedBy:      ch.UsedBy,
	}
}

func convertMedia(media []params.Media) []Media {
	result := make([]Media, len(media))
	for i, m := range media {
		result[i] = Media(m)
	}
	return result
}

func convertCategories(categories []params.Category) []Category {
	result := make([]Category, len(categories))
	for i, c := range categories {
		result[i] = Category(c)
	}
	return result
}

func convertChannelMap(cms []params.ChannelMap) []ChannelMap {
	result := make([]ChannelMap, len(cms))
	for i, cm := range cms {
		result[i] = convertOneChannelMap(cm)
	}
	return result
}

func convertOneChannelMap(defaultMap params.ChannelMap) ChannelMap {
	return ChannelMap{
		Channel: Channel{
			Name:       defaultMap.Channel.Name,
			Platform:   Platform(defaultMap.Channel.Platform),
			ReleasedAt: defaultMap.Channel.ReleasedAt,
		},
		Revision: Revision{
			CreatedAt:    defaultMap.Revision.CreatedAt,
			ConfigYAML:   defaultMap.Revision.ConfigYAML,
			Download:     Download(defaultMap.Revision.Download),
			MetadataYAML: defaultMap.Revision.MetadataYAML,
			Revision:     defaultMap.Revision.Revision,
			Version:      defaultMap.Revision.Version,
			Platforms:    convertPlatforms(defaultMap.Revision.Platforms),
		},
	}
}

func convertPlatforms(platforms []params.Platform) []Platform {
	result := make([]Platform, len(platforms))
	for i, p := range platforms {
		result[i] = Platform(p)
	}
	return result
}

// Although InfoResponse or FindResponse are similar, they will change once the
// charmhub API has settled.

type InfoResponse struct {
	Type           string       `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Charm          Charm        `json:"charm"`
	ChannelMap     []ChannelMap `json:"channel-map"`
	DefaultRelease ChannelMap   `json:"default-release,omitempty"`
}

type FindResponse struct {
	Type           string       `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Charm          Charm        `json:"charm"`
	ChannelMap     []ChannelMap `json:"channel-map"`
	DefaultRelease ChannelMap   `json:"default-release,omitempty"`
}

type ChannelMap struct {
	Channel  Channel  `json:"channel,omitempty"`
	Revision Revision `json:"revision,omitempty"`
}

type Channel struct {
	Name       string   `json:"name"` // track/risk
	Platform   Platform `json:"platform"`
	ReleasedAt string   `json:"released-at"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Series       string `json:"series"`
}

type Revision struct {
	CreatedAt    string     `json:"created-at"`
	ConfigYAML   string     `json:"config-yaml"`
	Download     Download   `json:"download"`
	MetadataYAML string     `json:"metadata-yaml"`
	Platforms    []Platform `json:"platforms"`
	Revision     int        `json:"revision"`
	Version      string     `json:"version"`
}

type Download struct {
	HashSHA265 string `json:"hash-sha-265"`
	Size       int    `json:"size"`
	URL        string `json:"url"`
}

type Charm struct {
	Categories  []Category        `json:"categories"`
	Description string            `json:"description"`
	License     string            `json:"license"`
	Media       []Media           `json:"media"`
	Publisher   map[string]string `json:"publisher"`
	Summary     string            `json:"summary"`
	UsedBy      []string          `json:"used-by"` // bundles which use the charm
}

type Media struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type Category struct {
	Featured bool   `json:"featured"`
	Name     string `json:"name"`
}
