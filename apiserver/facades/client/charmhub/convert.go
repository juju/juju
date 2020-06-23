// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
)

func convertCharmInfoResult(info charmhub.InfoResponse) params.InfoResponse {
	return params.InfoResponse{
		Type:           info.Type,
		ID:             info.ID,
		Name:           info.Name,
		Charm:          convertCharm(info.Charm),
		ChannelMap:     convertChannelMap(info.ChannelMap),
		DefaultRelease: convertOneChannelMap(info.DefaultRelease),
	}
}

func convertCharm(ch charmhub.Charm) params.CharmHubCharm {
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

func convertCategories(categories []charmhub.Category) []params.Category {
	result := make([]params.Category, len(categories))
	for i, c := range categories {
		result[i] = params.Category(c)
	}
	return result
}

func convertMedia(media []charmhub.Media) []params.Media {
	result := make([]params.Media, len(media))
	for i, m := range media {
		result[i] = params.Media(m)
	}
	return result
}

func convertChannelMap(cms []charmhub.ChannelMap) []params.ChannelMap {
	result := make([]params.ChannelMap, len(cms))
	for i, cm := range cms {
		result[i] = convertOneChannelMap(cm)
	}
	return result
}

func convertOneChannelMap(defaultMap charmhub.ChannelMap) params.ChannelMap {
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

func convertPlatforms(platforms []charmhub.Platform) []params.Platform {
	result := make([]params.Platform, len(platforms))
	for i, p := range platforms {
		result[i] = params.Platform(p)
	}
	return result
}
