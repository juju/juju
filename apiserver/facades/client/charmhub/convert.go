// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
	coreseries "github.com/juju/juju/core/series"
)

func convertCharmInfoResult(info transport.InfoResponse) params.InfoResponse {
	ir := params.InfoResponse{
		Type:        string(info.Type),
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Entity.Description,
		Publisher:   publisher(info.Entity),
		Summary:     info.Entity.Summary,
		Tags:        categories(info.Entity.Categories),
		StoreURL:    info.Entity.StoreURL,
	}
	var isKub bool
	switch transport.Type(ir.Type) {
	case transport.BundleType:
		ir.Bundle = convertBundle(info.Entity.Charms)
		// TODO (stickupkid): Get the Bundle.Release and set it to the
		// InfoResponse at a high level.
	case transport.CharmType:
		ir.Charm, ir.Series, isKub = convertCharm(info)
	}

	ir.Tracks, ir.Channels = transformInfoChannelMap(info.ChannelMap, isKub)
	return ir
}

func categories(cats []transport.Category) []string {
	if len(cats) == 0 {
		return nil
	}
	result := make([]string, len(cats))
	for i, v := range cats {
		result[i] = v.Name
	}
	return result
}

func convertCharmFindResults(responses []transport.FindResponse) []params.FindResponse {
	results := make([]params.FindResponse, len(responses))
	for k, response := range responses {
		results[k] = convertCharmFindResult(response)
	}
	return results
}

func convertCharmFindResult(resp transport.FindResponse) params.FindResponse {
	result := params.FindResponse{
		Type:      string(resp.Type),
		ID:        resp.ID,
		Name:      resp.Name,
		Publisher: publisher(resp.Entity),
		Summary:   resp.Entity.Summary,
		Version:   resp.DefaultRelease.Revision.Version,
		StoreURL:  resp.Entity.StoreURL,
	}
	supported := transformFindArchitectureSeries(resp.DefaultRelease)
	result.Arches, result.OS, result.Series = supported.Architectures, supported.OS, supported.Series
	return result
}

func publisher(ch transport.Entity) string {
	publisher, _ := ch.Publisher["display-name"]
	return publisher
}

// supported defines a tuple of extracted items from a platform.
type supported struct {
	Architectures []string
	OS            []string
	Series        []string
}

// transformFindArchitectureSeries returns a supported type which contains
// architectures and series for a given channel map.
func transformFindArchitectureSeries(channel transport.FindChannelMap) supported {
	if len(channel.Revision.Bases) < 1 {
		return supported{}
	}

	var (
		arches = set.NewStrings()
		os     = set.NewStrings()
		series = set.NewStrings()
	)
	for _, p := range channel.Revision.Bases {
		arches.Add(p.Architecture)
		os.Add(p.Name)
		// TODO hml - for this to be correct, must determine IsKubernetes from metadata.
		s, _ := coreseries.VersionSeries(p.Channel)
		series.Add(s)
	}
	return supported{
		Architectures: arches.SortedValues(),
		OS:            os.SortedValues(),
		Series:        series.SortedValues(),
	}
}

// transformInfoChannelMap returns channel map data in a format that facilitates
// determining track order and open vs closed channels for displaying channel
// data.
func transformInfoChannelMap(channelMap []transport.InfoChannelMap, isKub bool) ([]string, map[string]params.Channel) {
	var trackList []string

	seen := set.NewStrings("")
	channels := make(map[string]params.Channel, len(channelMap))

	for _, cm := range channelMap {
		ch := cm.Channel
		// Per the charmhub/snap channel spec.
		if ch.Track == "" {
			ch.Track = "latest"
		}
		chName := ch.Track + "/" + ch.Risk
		channels[chName] = params.Channel{
			Revision:   cm.Revision.Revision,
			ReleasedAt: ch.ReleasedAt,
			Risk:       ch.Risk,
			Track:      ch.Track,
			Size:       cm.Revision.Download.Size,
			Version:    cm.Revision.Version,
			Platforms:  convertBasesToPlatforms(cm.Revision.Bases, isKub),
		}
		if !seen.Contains(ch.Track) {
			seen.Add(ch.Track)
			trackList = append(trackList, ch.Track)
		}

	}
	return trackList, channels
}

// convertBasesToPlatforms converts a base to a platform.  Is mean to be a short
// term solution due to schedule.  It should be removed once platforms are
// replaced with bases through out the code.
func convertBasesToPlatforms(in []transport.Base, isKub bool) []params.Platform {
	out := make([]params.Platform, len(in))
	for i, v := range in {
		var series string
		if isKub {
			series = "kubernetes"
		} else {
			series, _ = coreseries.VersionSeries(v.Channel)
		}
		os, _ := coreseries.GetOSFromSeries(series)
		out[i] = params.Platform{
			Architecture: v.Architecture,
			OS:           strings.ToLower(os.String()),
			Series:       series,
		}
	}
	return out
}

func convertCharm(info transport.InfoResponse) (*params.CharmHubCharm, []string, bool) {
	charmHubCharm := params.CharmHubCharm{
		UsedBy: info.Entity.UsedBy,
	}
	var series []string
	var isKub bool
	if meta := unmarshalCharmMetadata(info.DefaultRelease.Revision.MetadataYAML); meta != nil {
		charmHubCharm.Subordinate = meta.Subordinate
		charmHubCharm.Relations = transformRelations(meta.Requires, meta.Provides)
		series = meta.ComputedSeries()
		isKub = isKubernetes(meta)
	}
	if cfg := unmarshalCharmConfig(info.DefaultRelease.Revision.ConfigYAML); cfg != nil {
		charmHubCharm.Config = params.ToCharmOptionMap(cfg)
	}
	return &charmHubCharm, series, isKub
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

func formatRelationPart(r map[string]charm.Relation) (map[string]string, bool) {
	if len(r) <= 0 {
		return nil, false
	}
	relations := make(map[string]string, len(r))
	for k, v := range r {
		relations[k] = v.Interface
	}
	return relations, true
}

func convertBundle(charms []transport.Charm) *params.CharmHubBundle {
	bundle := &params.CharmHubBundle{
		Charms: make([]params.BundleCharm, len(charms)),
	}
	for i, v := range charms {
		bundle.Charms[i] = params.BundleCharm{
			Name:      v.Name,
			PackageID: v.PackageID,
		}
	}
	return bundle
}

// TODO (hml) 2021-04-08
// Update location of series for kubernetes series once manifest.yaml
// is available.
func isKubernetes(meta *charm.Meta) bool {
	if len(meta.Containers) > 0 {
		return true
	}
	seriesSet := set.NewStrings(meta.Series...)
	if seriesSet.Contains("kubernetes") {
		return true
	}
	return false
}
