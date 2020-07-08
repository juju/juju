// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
)

func convertCharmInfoResult(info transport.InfoResponse) params.InfoResponse {
	ir := params.InfoResponse{
		Type:        info.Type,
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Entity.Description,
		Publisher:   publisher(info.Entity),
		Summary:     info.Entity.Summary,
	}
	switch ir.Type {
	case "Bundle":
		ir.Bundle = convertBundle()
	case "Charm":
		ir.Charm = convertCharm(info)
	}
	ir.Tracks, ir.Channels = transformChannelMap(info.ChannelMap)
	return ir
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
		Type:   resp.Type,
		ID:     resp.ID,
		Name:   resp.Name,
		Entity: convertEntity(resp.Entity),
		//DefaultRelease: convertOneChannelMap(resp.DefaultRelease),
	}
}

func convertEntity(ch transport.Entity) params.CharmHubEntity {
	return params.CharmHubEntity{
		//Categories:  convertCategories(ch.Categories),
		Description: ch.Description,
		License:     ch.License,
		//Media:       convertMedia(ch.Media),
		Publisher: ch.Publisher,
		Summary:   ch.Summary,
		UsedBy:    ch.UsedBy,
	}
}

func publisher(ch transport.Entity) string {
	publisher, _ := ch.Publisher["display-name"]
	return publisher
}

// transformChannelMap returns channel map data in a format that facilitates
// determining track order and open vs closed channels for displaying channel
// data.
func transformChannelMap(channelMap []transport.ChannelMap) ([]string, map[string]params.Channel) {
	trackList := []string{}
	seen := set.NewStrings("")
	channels := make(map[string]params.Channel, len(channelMap))
	for _, cm := range channelMap {
		ch := cm.Channel
		chName := ch.Track + "/" + ch.Risk
		channels[chName] = params.Channel{
			Revision:   cm.Revision.Revision,
			ReleasedAt: ch.ReleasedAt,
			Risk:       ch.Risk,
			Track:      ch.Track,
			Size:       cm.Revision.Download.Size,
			Version:    cm.Revision.Version,
		}
		if !seen.Contains(ch.Track) {
			seen.Add(ch.Track)
			trackList = append(trackList, ch.Track)
		}
	}
	return trackList, channels
}

func convertCharm(info transport.InfoResponse) params.CharmHubCharm {
	charmHubCharm := params.CharmHubCharm{
		UsedBy: info.Entity.UsedBy,
	}
	if meta := unmarshalCharmMetadata(info.DefaultRelease.Revision.MetadataYAML); meta != nil {
		charmHubCharm.Subordinate = meta.Subordinate
		charmHubCharm.Tags = meta.Tags
		charmHubCharm.Relations = transformRelations(meta.Requires, meta.Provides)
	}
	if cfg := unmarshalCharmConfig(info.DefaultRelease.Revision.ConfigYAML); cfg != nil {
		charmHubCharm.Config = params.ToCharmOptionMap(cfg)
	}
	return charmHubCharm
}

func unmarshalCharmMetadata(metadataYAML string) *charm.Meta {
	if metadataYAML == "" {
		return nil
	}
	m := metadataYAML
	meta, err := charm.ReadMeta(bytes.NewBufferString(m))
	if err != nil {
		// Do not fail on unmarshalling metadata, log instead.
		// This should not happen, however at implementation
		// we were dealing with handwritten data for test, not
		// the real deal.  Usually charms are validated before
		// being uploaded to the store.
		logger.Warningf(errors.Annotate(err, "cannot unmarshal charm metadata").Error())
		return nil
	}
	return meta
}

func unmarshalCharmConfig(configYAML string) *charm.Config {
	if configYAML == "" {
		return nil
	}
	cfgYaml := configYAML
	cfg, err := charm.ReadConfig(bytes.NewBufferString(cfgYaml))
	if err != nil {
		// Do not fail on unmarshalling metadata, log instead.
		// This should not happen, however at implementation
		// we were dealing with handwritten data for test, not
		// the real deal.  Usually charms are validated before
		// being uploaded to the store.
		logger.Warningf(errors.Annotate(err, "cannot unmarshal charm config").Error())
		return nil
	}
	return cfg
}

func transformRelations(requires, provides map[string]charm.Relation) map[string]map[string]string {
	if len(requires) == 0 && len(provides) == 0 {
		logger.Debugf("no relation data found in charm meta data")
		return nil
	}
	relations := make(map[string]map[string]string)
	if provides, ok := formatRelationPart(provides); ok {
		relations["provides"] = provides
	}
	if requires, ok := formatRelationPart(requires); ok {
		relations["requires"] = requires
	}
	return relations
}

func formatRelationPart(rels map[string]charm.Relation) (map[string]string, bool) {
	if len(rels) <= 0 {
		return nil, false
	}
	relations := make(map[string]string, len(rels))
	for k, v := range rels {
		relations[k] = v.Interface
	}
	return relations, true
}

func convertBundle() params.CharmHubBundle {
	// TODO (hml) 2020-07-06
	// Implemented once how to get charms in a bundle is defined by the api.
	return params.CharmHubBundle{}
}
