// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/selector"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
)

// CharmHubClient represents the methods required of a
// client to install or upgrade a CharmHub charm.
type CharmHubClient interface {
	DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...charmhub.DownloadOption) (*charm.CharmArchive, error)
	Info(ctx context.Context, name string, options ...charmhub.InfoOption) (transport.InfoResponse, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type chRepo struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel call the CharmHub version of
// ResolveWithPreferredChannel.
func (c *chRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmHub charm %q", curl)

	channel, err := makeChannel(origin)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	platform, err := makePlatform(origin)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	// In order to get the metadata for a given charm we need to ensure that
	// we ask for the channel otherwise the metadata won't show up.
	var options []charmhub.InfoOption
	if s := channel.String(); s != "" {
		options = append(options, charmhub.WithChannel(s))
	}

	info, err := c.client.Info(context.TODO(), curl.Name, options...)
	if err != nil {
		// Improve error message here
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	// If no revision nor channel specified, use the default release.
	if curl.Revision == -1 && channel.String() == "" {
		logger.Debugf("Resolving charm with default release")
		resURL, resOrigin, serie, err := c.resolveViaChannelMap(info.Type, curl, origin, info.DefaultRelease, true)
		if err != nil {
			return nil, params.CharmOrigin{}, nil, errors.Trace(err)
		}

		resOrigin.ID = info.ID
		outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
		if err != nil {
			return nil, params.CharmOrigin{}, nil, errors.Trace(err)
		}
		return resURL, outputOrigin, serie, nil
	}

	logger.Debugf("Resolving charm with revision %d and/or channel %s and origin %s", curl.Revision, channel.String(), origin)

	preferred := channelPlatform{
		Channel:  channel,
		Platform: platform,
	}
	channelMap, overrideArch, err := findChannelMap(curl.Revision, preferred, info.ChannelMap)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}
	resURL, resOrigin, series, err := c.resolveViaChannelMap(info.Type, curl, origin, channelMap, overrideArch)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	resOrigin.ID = info.ID
	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}
	return resURL, outputOrigin, series, nil
}

// DownloadCharm downloads the provided download URL from CharmHub using the
// provided archive path.
// A charm archive is returned.
func (c *chRepo) DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error) {
	logger.Debugf("DownloadCharm from CharmHub %q", resourceURL)
	curl, err := url.Parse(resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.client.DownloadAndRead(context.TODO(), curl, archivePath)
}

// FindDownloadURL returns the url from which to download the CharmHub
// charm defined by the provided curl and charm origin.  An updated
// charm origin is also returned with the ID and hash for the charm
// to be downloaded.  If the provided charm origin has no ID, it is
// assumed that the charm is being installed, not refreshed.
func (c *chRepo) FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	logger.Tracef("FindDownloadURL %v %v", curl, origin)
	if origin.Type == "bundle" {
		durl, resOrigin, err := c.findBundleDownloadURL(curl, origin)
		if err != nil {
			return nil, corecharm.Origin{}, errors.Trace(err)
		}
		outputOrigin, err := sanitizeCoreCharmOrigin(resOrigin, origin)
		return durl, outputOrigin, errors.Trace(err)
	}

	cfg, err := refreshConfig(curl, origin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	logger.Debugf("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return nil, corecharm.Origin{}, errors.Errorf("more than 1 result found")
	}
	findResult := result[0]
	if findResult.Error != nil {
		// TODO: (hml) 4-sep-2020
		// When list of error codes available, create real error for them.
		return nil, corecharm.Origin{}, errors.Errorf("%s: %s", findResult.Error.Code, findResult.Error.Message)
	}

	resOrigin := origin
	resOrigin.ID = findResult.Entity.ID
	resOrigin.Hash = findResult.Entity.Download.HashSHA256

	durl, err := url.Parse(findResult.Entity.Download.URL)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	outputOrigin, err := sanitizeCoreCharmOrigin(resOrigin, origin)
	return durl, outputOrigin, errors.Trace(err)
}

func (c *chRepo) findBundleDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	info, err := c.client.Info(context.TODO(), curl.Name)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	logger.Debugf("Locate bundle using: %v", origin)

	selector := selector.NewSelectorForBundle(series.SupportedJujuControllerSeries())
	return selector.Locate(info, origin)
}

func (c *chRepo) resolveViaChannelMap(t transport.Type, curl *charm.URL, origin params.CharmOrigin, channelMap transport.InfoChannelMap, overrideArch bool) (*charm.URL, params.CharmOrigin, []string, error) {
	mapChannel := channelMap.Channel
	mapRevision := channelMap.Revision

	curl.Revision = mapRevision.Revision

	origin.Type = t
	origin.Revision = &mapRevision.Revision
	origin.Risk = mapChannel.Risk
	origin.Track = &mapChannel.Track

	// This is a work around for the fact that the charmhub API can return the
	// wrong arch that we're looking for. An example being that we searched for
	// amd64, but the channel.platform is set to s390x, yet the
	// revision.platforms contains "all". In this instance we get back s390x,
	// even though we know the exact same revision will work with everything.
	// This is a limited work around until the charmhub API will correctly
	// explode the channel map architecture.
	if !overrideArch {
		origin.Architecture = mapChannel.Platform.Architecture
	}
	origin.OS = mapChannel.Platform.OS
	origin.Series = mapChannel.Platform.Series

	// The metadata is empty, this can happen if we've requested something from
	// the charmhub API that we didn't provide the right hint for (channel or
	// revision).
	// Eventually we should drop the computed series for charmhub requests and
	// only use the API to tell us which series we target. Until that happens
	// we should fallback to one we do know and won't cause the deployment to
	// fail.
	var (
		err  error
		meta Metadata
	)
	switch t {
	case "charm":
		if mapRevision.MetadataYAML == "" {
			logger.Warningf("No metadata yaml found, using fallback computed series for %q.", curl)
			return curl, origin, []string{origin.Series}, nil
		}

		meta, err = unmarshalCharmMetadata(mapRevision.MetadataYAML)
	case "bundle":
		if mapRevision.BundleYAML == "" {
			logger.Warningf("No bundle yaml found, using fallback computed series for %q.", curl)
			return curl, origin, []string{origin.Series}, nil
		}

		meta, err = unmarshalBundleMetadata(mapRevision.BundleYAML)
	default:
		err = errors.Errorf("unexpected charm/bundle type %q", t)
	}
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "cannot unmarshal charm metadata")
	}
	return curl, origin, meta.ComputedSeries(), nil
}

type channelPlatform struct {
	Channel  corecharm.Channel
	Platform corecharm.Platform
}

func (cp channelPlatform) Match(other transport.InfoChannelMap) (bool, bool) {
	if !cp.MatchChannel(other) {
		return false, false
	}

	return cp.MatchArch(other)
}

func (cp channelPlatform) MatchChannel(other transport.InfoChannelMap) bool {
	return cp.Channel.Normalize().String() == other.Channel.Name
}

func (cp channelPlatform) MatchArch(other transport.InfoChannelMap) (bool, bool) {
	if other.Channel.Platform.Architecture == "all" {
		return true, true
	}
	norm := cp.Platform.Normalize()
	if norm.Architecture == other.Channel.Platform.Architecture {
		return false, true
	}

	for _, platform := range other.Revision.Platforms {
		if platform.Architecture == "all" {
			return true, true
		}
		if platform.Architecture == norm.Architecture {
			return false, true
		}
	}
	return false, false
}

// refreshConfig creates a RefreshConfig for the given input.
// If the origin.ID is not set, a install refresh config is returned. For
//   install. Channel and Revision are mutually exclusive in the api, only
//   one will be used.  Channel first, Revision is a fallback.
// If the origin.ID is set, a refresh config is returned.
func refreshConfig(curl *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	var rev int
	if origin.Revision != nil {
		rev = *origin.Revision
	}
	var channel string
	if origin.Channel != nil {
		channel = origin.Channel.String()
	}
	if origin.Revision == nil && origin.Channel == nil && origin.ID == "" {
		channel = corecharm.DefaultChannelString
	}

	var (
		cfg charmhub.RefreshConfig
		err error

		platform = charmhub.RefreshPlatform{
			Architecture: origin.Platform.Architecture,
			OS:           origin.Platform.OS,
			Series:       origin.Platform.Series,
		}
	)
	switch {
	case origin.ID == "" && channel != "":
		// If there is no origin ID, we haven't downloaded this charm before.
		// Try channel first.
		cfg, err = charmhub.InstallOneFromChannel(curl.Name, channel, platform)
	case origin.ID == "" && channel == "":
		// If there is no origin ID, we haven't downloaded this charm before.
		// No channel, try with revision.
		cfg, err = charmhub.InstallOneFromRevision(curl.Name, rev, platform)
	case origin.ID != "":
		// This must be a charm upgrade if we have an ID.  Use the refresh action
		// for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(origin.ID, rev, channel, platform)
	default:
		return nil, errors.NotValidf("origin %v", origin)
	}
	return cfg, err
}

func makeChannel(origin params.CharmOrigin) (corecharm.Channel, error) {
	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	if track == "" && origin.Risk == "" {
		return corecharm.Channel{}, nil
	}
	ch, err := corecharm.MakeChannel(track, origin.Risk, "")
	if err != nil {
		return corecharm.Channel{}, errors.Trace(err)
	}
	return ch.Normalize(), nil
}

func makePlatform(origin params.CharmOrigin) (corecharm.Platform, error) {
	p, err := corecharm.MakePlatform(origin.Architecture, origin.OS, origin.Series)
	if err != nil {
		return p, errors.Trace(err)
	}
	return p.Normalize(), nil
}

func findChannelMap(rev int, preferred channelPlatform, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, bool, error) {
	if len(channelMaps) == 0 {
		return transport.InfoChannelMap{}, false, errors.NotValidf("no channels provided by CharmHub")
	}
	switch {
	case preferred.Channel.String() != "" && rev != -1:
		return findByRevisionAndChannel(rev, preferred, channelMaps)
	case preferred.Channel.String() != "":
		return findByChannel(preferred, channelMaps)
	default: // rev != -1
		return findByRevision(rev, preferred, channelMaps)
	}
}

func findByRevision(rev int, preferred channelPlatform, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, bool, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev {
			if overrideArch, ok := preferred.MatchArch(cMap); ok {
				// Channel map is in order of most newest/stable channel,
				// return the first of the requested revision.
				return cMap, overrideArch, nil
			}
		}
	}
	return transport.InfoChannelMap{}, false, errors.NotFoundf("charm revision %d", rev)
}

func findByChannel(preferred channelPlatform, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, bool, error) {
	for _, cMap := range channelMaps {
		if overrideArch, ok := preferred.Match(cMap); ok {
			return cMap, overrideArch, nil
		}
	}
	return transport.InfoChannelMap{}, false, errors.NotFoundf("channel %q with arch %q", preferred.Channel.String(), preferred.Platform.Architecture)
}

func findByRevisionAndChannel(rev int, preferred channelPlatform, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, bool, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev {
			if overrideArch, ok := preferred.Match(cMap); ok {
				return cMap, overrideArch, nil
			}
		}
	}
	return transport.InfoChannelMap{}, false, errors.NotFoundf("charm revision %d for channel %q with arch %q", rev, preferred.Channel.String(), preferred.Platform.Architecture)
}
