// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub/transport"
)

func convertInfoResponse(info transport.InfoResponse, arch string, base corebase.Base) (InfoResponse, error) {
	ir := InfoResponse{
		Type:        string(info.Type),
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Entity.Description,
		Publisher:   publisher(info.Entity),
		Summary:     info.Entity.Summary,
		Tags:        categories(info.Entity.Categories),
		StoreURL:    info.Entity.StoreURL,
	}
	switch transport.Type(ir.Type) {
	case transport.BundleType:
		ir.Bundle = convertBundle(info.Entity.Charms)
		// TODO (stickupkid): Get the Bundle.Release and set it to the
		// InfoResponse at a high level.
	case transport.CharmType:
		ir.Charm = convertCharm(info)
	}

	seen := make(map[Base]bool)
	for _, b := range info.DefaultRelease.Revision.Bases {
		base := Base{Name: b.Name, Channel: b.Channel}
		if seen[base] {
			continue
		}
		seen[base] = true
		ir.Supports = append(ir.Supports, base)
	}

	var err error
	ir.Tracks, ir.Channels, err = filterChannels(info.ChannelMap, arch, base)
	if err != nil {
		return ir, errors.Trace(err)
	}
	return ir, nil
}

func convertCharmFindResults(responses []transport.FindResponse) []FindResponse {
	results := make([]FindResponse, len(responses))
	for i, resp := range responses {
		results[i] = convertCharmFindResult(resp)
	}
	return results
}

func convertCharmFindResult(resp transport.FindResponse) FindResponse {
	result := FindResponse{
		Type:      string(resp.Type),
		ID:        resp.ID,
		Name:      resp.Name,
		Publisher: publisher(resp.Entity),
		Summary:   resp.Entity.Summary,
		Version:   resp.DefaultRelease.Revision.Version,
		StoreURL:  resp.Entity.StoreURL,
	}
	supported := transformFindArchitectureSeries(resp.DefaultRelease)
	result.Arches, result.OS, result.Supports = supported.Architectures, supported.OS, supported.Supports
	return result
}

func convertBundle(charms []transport.Charm) *Bundle {
	bundle := &Bundle{
		Charms: make([]BundleCharm, len(charms)),
	}
	for i, v := range charms {
		bundle.Charms[i] = BundleCharm{
			Name: v.Name,
		}
	}
	return bundle
}

func convertCharm(info transport.InfoResponse) *Charm {
	ch := &Charm{
		UsedBy: info.Entity.UsedBy,
	}
	if meta := unmarshalCharmMetadata(info.DefaultRelease.Revision.MetadataYAML); meta != nil {
		ch.Subordinate = meta.Subordinate
		ch.Relations = transformRelations(meta.Requires, meta.Provides)
	}
	if cfg := unmarshalCharmConfig(info.DefaultRelease.Revision.ConfigYAML); cfg != nil {
		ch.Config = &charm.Config{
			Options: toCharmOptionMap(cfg),
		}
	}
	return ch
}

func includeChannel(p []corecharm.Platform, architecture string, base corebase.Base) bool {
	allArch := architecture == ArchAll

	// If we're searching for everything then we can skip the filtering logic
	// and return immediately.
	if allArch && base.Empty() {
		return true
	}

	archSet := channelArches(p)
	basesSet := channelBases(p)

	contains := func(bases []Base, base corebase.Base) bool {
		for _, b := range bases {
			if b.Name == base.OS && b.Channel == base.Channel.Track {
				return true
			}
		}
		return false
	}

	if (allArch || archSet.Contains(architecture)) &&
		(base.Empty() || contains(basesSet, base)) {
		return true
	}
	return false
}

func channelBases(platforms []corecharm.Platform) []Base {
	seen := make(map[Base]bool)
	var bases []Base
	for _, v := range platforms {
		base := Base{Name: v.OS, Channel: v.Channel}
		if seen[base] {
			continue
		}
		seen[base] = true
		bases = append(bases, base)
	}
	return bases
}

func channelArches(platforms []corecharm.Platform) set.Strings {
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

func publisher(ch transport.Entity) string {
	publisher, _ := ch.Publisher["display-name"]
	return publisher
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

// supported defines a tuple of extracted items from a platform.
type supported struct {
	Architectures []string
	OS            []string
	Supports      []Base
}

// transformFindArchitectureSeries returns a supported type which contains
// architectures and series for a given channel map.
func transformFindArchitectureSeries(channel transport.FindChannelMap) supported {
	if len(channel.Revision.Bases) < 1 {
		return supported{}
	}

	arches := set.NewStrings()
	os := set.NewStrings()
	var bases []Base
	basesSeen := make(map[Base]bool)
	for _, p := range channel.Revision.Bases {
		arches.Add(p.Architecture)
		os.Add(p.Name)

		base := Base{Name: p.Name, Channel: p.Channel}
		if basesSeen[base] {
			continue
		}
		basesSeen[base] = true
		bases = append(bases, base)
	}
	return supported{
		Architectures: arches.SortedValues(),
		OS:            os.SortedValues(),
		Supports:      bases,
	}
}

func toCharmOptionMap(config *charm.Config) map[string]charm.Option {
	if config == nil {
		return nil
	}
	result := make(map[string]charm.Option)
	for key, value := range config.Options {
		result[key] = toParamsCharmOption(value)
	}
	return result
}

func toParamsCharmOption(opt charm.Option) charm.Option {
	return charm.Option{
		Type:        opt.Type,
		Description: opt.Description,
		Default:     opt.Default,
	}
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
	if configYAML == "" || strings.TrimSpace(configYAML) == "{}" {
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

// filterChannels returns channel map data in a format that facilitates
// determining track order and open vs closed channels for displaying channel
// data. The result is filtered on base and arch.
func filterChannels(channelMap []transport.InfoChannelMap, arch string, base corebase.Base) ([]string, RevisionsMap, error) {
	var trackList []string

	tracksSeen := set.NewStrings()
	revisionsSeen := set.NewStrings()
	channels := make(RevisionsMap)

	for _, cm := range channelMap {
		ch := cm.Channel
		// Per the charmhub/snap channel spec.
		if ch.Track == "" {
			ch.Track = "latest"
		}
		if !tracksSeen.Contains(ch.Track) {
			tracksSeen.Add(ch.Track)
			trackList = append(trackList, ch.Track)
		}

		platforms := convertBasesToPlatforms(cm.Revision.Bases)
		if !includeChannel(platforms, arch, base) {
			continue
		}

		revisionKey := fmt.Sprintf("%s/%s:%d", ch.Track, ch.Risk, cm.Revision.Revision)
		if revisionsSeen.Contains(revisionKey) {
			continue
		}
		revisionsSeen.Add(revisionKey)

		revision := Revision{
			Track:      ch.Track,
			Risk:       ch.Risk,
			Version:    cm.Revision.Version,
			Revision:   cm.Revision.Revision,
			ReleasedAt: ch.ReleasedAt,
			Size:       cm.Revision.Download.Size,
			Arches:     channelArches(platforms).SortedValues(),
			Bases:      channelBases(platforms),
		}

		if _, ok := channels[ch.Track]; !ok {
			channels[ch.Track] = make(map[string][]Revision)
		}
		channels[ch.Track][ch.Risk] = append(channels[ch.Track][ch.Risk], revision)
	}

	for _, risks := range channels {
		for _, revisions := range risks {
			// Sort revisions by latest ReleasedAt first.
			sort.Slice(revisions, func(i, j int) bool {
				ti, _ := time.Parse(time.RFC3339, revisions[i].ReleasedAt)
				tj, _ := time.Parse(time.RFC3339, revisions[j].ReleasedAt)
				return ti.After(tj)
			})
		}
	}

	return trackList, channels, nil
}

func convertBasesToPlatforms(in []transport.Base) []corecharm.Platform {
	out := make([]corecharm.Platform, len(in))
	for i, v := range in {
		out[i] = corecharm.Platform{
			Architecture: v.Architecture,
			OS:           strings.ToLower(v.Name),
			Channel:      v.Channel,
		}
	}
	return out
}
