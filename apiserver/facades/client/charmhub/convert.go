// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
)

func convertCharmInfoResult(info transport.InfoResponse) params.InfoResponse {
	return params.InfoResponse{
		Type:           info.Type,
		ID:             info.ID,
		Name:           info.Name,
		Charm:          convertCharm(info.Charm),
		ChannelMap:     convertChannelMap(info.ChannelMap),
		DefaultRelease: convertOneChannelMap(info.DefaultRelease),
	}
}

func convertCharmFindResults(responses []transport.FindResponse) []params.FindResponse {
	results := make([]params.FindResponse, len(responses))
	for k, response := range responses {
		results[k] = convertCharmFindResult(response)
	}
	return results
}

func convertCharmFindResult(resp transport.FindResponse) params.FindResponse {
	return params.FindResponse{
		Type:           resp.Type,
		ID:             resp.ID,
		Name:           resp.Name,
		Charm:          convertCharm(resp.Charm),
		ChannelMap:     convertChannelMap(resp.ChannelMap),
		DefaultRelease: convertOneChannelMap(resp.DefaultRelease),
	}
}

func convertCharm(ch transport.Charm) params.CharmHubCharm {
	return params.CharmHubCharm{
		Categories:  convertCategories(ch.Categories),
		Description: ch.Description,
		License:     ch.License,
		Media:       convertMedia(ch.Media),
		Publisher:   ch.Publisher,
		Summary:     ch.Summary,
		UsedBy:      ch.UsedBy,
	}
}

func convertCategories(categories []transport.Category) []params.Category {
	result := make([]params.Category, len(categories))
	for i, c := range categories {
		result[i] = params.Category(c)
	}
	return result
}

func convertMedia(media []transport.Media) []params.Media {
	result := make([]params.Media, len(media))
	for i, m := range media {
		result[i] = params.Media(m)
	}
	return result
}

func convertChannelMap(cms []transport.ChannelMap) []params.ChannelMap {
	result := make([]params.ChannelMap, len(cms))
	for i, cm := range cms {
		result[i] = convertOneChannelMap(cm)
	}
	return result
}

func convertOneChannelMap(defaultMap transport.ChannelMap) params.ChannelMap {
	return params.ChannelMap{
		Channel: params.Channel{
			Name:       defaultMap.Channel.Name,
			Platform:   params.Platform(defaultMap.Channel.Platform),
			ReleasedAt: defaultMap.Channel.ReleasedAt,
		},
		Revision: params.Revision{
			CreatedAt:    defaultMap.Revision.CreatedAt,
			ConfigYAML:   defaultMap.Revision.ConfigYAML,
			Download:     params.Download(defaultMap.Revision.Download),
			MetadataYAML: defaultMap.Revision.MetadataYAML,
			Revision:     defaultMap.Revision.Revision,
			Version:      defaultMap.Revision.Version,
			Platforms:    convertPlatforms(defaultMap.Revision.Platforms),
		},
	}
}

func convertPlatforms(platforms []transport.Platform) []params.Platform {
	result := make([]params.Platform, len(platforms))
	for i, p := range platforms {
		result[i] = params.Platform(p)
	}
	return result
}
