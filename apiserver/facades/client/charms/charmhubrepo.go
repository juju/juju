// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"

	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
)

// CharmHubClient represents the methods required of a
// client to install or upgrade a CharmHub charm.
type CharmHubClient interface {
	DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...charmhub.DownloadOption) (*charm.CharmArchive, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type chRepo struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel defines a way using the given charm URL and
// charm origin (platform and channel) to locate a matching charm against the
// Charmhub API.
//
// There are a few things to note in the attempt to resolve the charm and it's
// supporting series.
//
//    1. The algorithm for this is terrible. For charmstore lookups, only one
//       request is required, unfortunately for Charmhub the worst case for this
//       will be 2.
//       Most of the initial requests from the client will hit this first time
//       around (think `juju deploy foo`) without a series (client can then
//       determine what to call the real request with) will be default of 2
//       requests.
//    2. Attempting to find the default series will require 2 requests so that
//       we can find the correct charm ID ensure that the default series exists
//       along with the revision.
//    3. In theory we could just return most of this information without the
//       re-request, but we end up with missing data and potential incorrect
//       charm downloads later.
//
// When charmstore goes, we could potentially rework how the client requests
// the store.
func (c *chRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmHub charm %q with origin %v", curl, origin)

	if curl.Revision != -1 {
		return nil, params.CharmOrigin{}, nil, errors.Errorf("specifying a revision is not supported, please use a channel.")
	}

	res, err := c.refreshOne(curl, apicharm.APICharmOrigin(origin).CoreCharmOrigin())
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh")
	}

	if resErr := res.Error; resErr != nil {
		switch resErr.Code {
		case transport.ErrorCodeInvalidCharmPlatform:
			logger.Tracef("Invalid charm platform %q %v - Default Platforms: %v", curl, origin, resErr.Extra.DefaultPlatforms)
			platform, err := c.selectNextPlatform(resErr.Extra.DefaultPlatforms, origin)
			if err != nil {
				return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh")
			}

			origin.OS = platform.OS
			origin.Series = platform.Series

		case transport.ErrorCodeRevisionNotFound:
			logger.Tracef("Revision not found %q %v - Default Platforms: %v", curl, origin, resErr.Extra.Releases)
			release, err := c.selectNextRelease(resErr.Extra.Releases, origin)
			if err != nil {
				return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh")
			}

			origin.OS = release.OS
			origin.Series = release.Series

		default:
			return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh error: %s", resErr.Message)
		}
		if origin.Series == "" {
			return nil, params.CharmOrigin{}, nil, errors.NotValidf("series for %s", curl.Name)
		}

		res, err = c.refreshOne(curl, apicharm.APICharmOrigin(origin).CoreCharmOrigin())
		if err != nil {
			return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh retry")
		}
		if resErr := res.Error; resErr != nil {
			return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh retry: %s", resErr.Message)
		}
	}

	// Use the channel that was actually picked by the API. This should
	// account for the closed tracks in a given channel.
	channel, err := corecharm.ParseChannelNormalize(res.EffectiveChannel)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "invalid channel")
	}

	var track *string
	if channel.Track != "" {
		track = &channel.Track
	}

	entity := res.Entity

	// Ensure we send the updated curl back, with all the correct segments.
	revision := entity.Revision
	resCurl := curl.
		WithSeries(origin.Series).
		WithArchitecture(origin.Architecture).
		WithRevision(revision)

	resOrigin := origin
	resOrigin.Type = string(entity.Type)
	resOrigin.ID = res.ID
	resOrigin.Hash = entity.Download.HashSHA256
	resOrigin.Track = track
	resOrigin.Risk = string(channel.Risk)
	resOrigin.Revision = &revision

	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}
	logger.Tracef("Resolved CharmHub charm %q with origin %v", resCurl, outputOrigin)
	return resCurl, outputOrigin, []string{outputOrigin.Series}, nil
}

// DownloadCharm downloads the provided download URL from CharmHub using the
// provided archive path.
// A charm archive is returned.
func (c *chRepo) DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error) {
	logger.Tracef("DownloadCharm from CharmHub %q", resourceURL)
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

	refreshRes, err := c.refreshOne(curl, origin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	if refreshRes.Error != nil {
		return nil, corecharm.Origin{}, errors.Errorf("%s: %s", refreshRes.Error.Code, refreshRes.Error.Message)
	}

	resOrigin := origin
	resOrigin.ID = refreshRes.Entity.ID
	resOrigin.Hash = refreshRes.Entity.Download.HashSHA256

	durl, err := url.Parse(refreshRes.Entity.Download.URL)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	outputOrigin, err := sanitizeCoreCharmOrigin(resOrigin, origin)
	return durl, outputOrigin, errors.Trace(err)
}

func (c *chRepo) refreshOne(curl *charm.URL, origin corecharm.Origin) (transport.RefreshResponse, error) {
	cfg, err := refreshConfig(curl, origin)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	logger.Tracef("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return transport.RefreshResponse{}, errors.Errorf("more than 1 result found")
	}

	return result[0], nil
}

func (c *chRepo) selectNextPlatform(platforms []transport.Platform, origin params.CharmOrigin) (transport.Platform, error) {
	if len(platforms) == 0 {
		return transport.Platform{}, errors.Errorf("no platforms available")
	}
	// We've got a invalid charm platform error, the error should contain
	// a valid platform to query again to get the right information. If
	// the platform is empty, consider it a failure.
	var (
		found    bool
		platform transport.Platform
	)
	for _, platform = range platforms {
		if platform.Architecture != origin.Architecture {
			continue
		}
		found = true
		break
	}
	if !found {
		return transport.Platform{}, errors.NotFoundf("platform")
	}
	return platform, nil
}

func (c *chRepo) selectNextRelease(releases []transport.Release, origin params.CharmOrigin) (Release, error) {
	if len(releases) == 0 {
		return Release{}, errors.Errorf("no releases available")
	}
	if origin.Series == "" {
		// If the origin is empty, then we want to help the user out
		// by display a series of suggestions to try.
		suggestions := composeSuggestions(releases, origin)
		var s string
		if len(suggestions) > 0 {
			s = fmt.Sprintf("; suggestions: %v", strings.Join(suggestions, ", "))
		}
		return Release{}, errors.Errorf("no charm or bundle matching channel or platform%s", s)
	}

	// From the suggestion list, go look up a release that we can retry.
	return selectReleaseByArchAndChannel(releases, origin)
}

// Method describes the method for requesting the charm using the RefreshAPI.
type Method string

const (
	// MethodRevision utilizes requesting by the revision only.
	MethodRevision Method = "revision"
	// MethodChannel utilizes requesting by the channel only.
	MethodChannel Method = "channel"
	// MethodID utilizes requesting by the id and channel (falls back to
	// latest/stable if channel is found).
	MethodID Method = "id"
)

// refreshConfig creates a RefreshConfig for the given input.
// If the origin.ID is not set, a install refresh config is returned. For
//   install. Channel and Revision are mutually exclusive in the api, only
//   one will be used.  Channel first, Revision is a fallback.
// If the origin.ID is set, a refresh config is returned.
func refreshConfig(curl *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	// Work out the correct install method.
	var rev int
	var method Method
	if origin.Revision != nil && *origin.Revision >= 0 {
		rev = *origin.Revision
		method = MethodRevision
	}

	var (
		channel         string
		nonEmptyChannel = origin.Channel != nil && !origin.Channel.Empty()
	)
	if method == MethodRevision && nonEmptyChannel {
		return nil, errors.NotValidf("supplying both revision and channel")
	}

	// Select the appropriate channel based on the supplied origin.
	if nonEmptyChannel {
		channel = origin.Channel.String()
	} else if method != MethodRevision {
		channel = corecharm.DefaultChannelString
	}

	if origin.ID == "" && channel != "" {
		method = MethodChannel
	}
	// Bundles can not use method IDs, which in turn forces a refresh.
	if method == MethodRevision && !matchesType(origin.Type, transport.BundleType) && origin.ID != "" {
		method = MethodID
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
	switch method {
	case MethodChannel:
		// Install from just the name and the channel. If there is no origin ID,
		// we haven't downloaded this charm before.
		// Try channel first.
		cfg, err = charmhub.InstallOneFromChannel(curl.Name, channel, platform)
	case MethodRevision:
		// If there is a revision, install it using that. If there is no origin
		// ID, we haven't downloaded this charm before.
		// No channel, try with revision.
		cfg, err = charmhub.InstallOneFromRevision(curl.Name, rev, platform)
	case MethodID:
		// This must be a charm upgrade if we have an ID.  Use the refresh
		// action for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(origin.ID, rev, channel, platform)
	default:
		return nil, errors.NotValidf("origin %v", origin)
	}
	return cfg, err
}

func matchesType(originType string, expected transport.Type) bool {
	return string(expected) == originType
}

func composeSuggestions(releases []transport.Release, origin params.CharmOrigin) []string {
	var suggestions []string
	for _, release := range releases {
		platform := release.Platform
		arch, os, series := platform.Architecture, platform.OS, platform.Series
		if arch == "all" {
			arch = origin.Architecture
		}
		if arch != origin.Architecture {
			continue
		}
		if os == "all" {
			os = origin.OS
		}
		if series == "all" {
			series = origin.Series
		}
		suggestions = append(suggestions, fmt.Sprintf("%s with %s/%s/%s", release.Channel, arch, os, series))
	}
	return suggestions
}

// Release represents a release that a charm can be selected from.
type Release struct {
	OS, Series string
}

func selectReleaseByArchAndChannel(releases []transport.Release, origin params.CharmOrigin) (Release, error) {
	for _, release := range releases {
		platform := release.Platform

		var track string
		if origin.Track != nil {
			track = *origin.Track
		}
		var channel *corecharm.Channel
		if origin.Risk != "" {
			c, err := corecharm.MakeChannel(track, origin.Risk, "")
			if err != nil {
				continue
			}
			c = c.Normalize()
			channel = &c
		}

		arch, os, series := platform.Architecture, platform.OS, platform.Series
		if (channel == nil || channel.String() == release.Channel) && (arch == "all" || arch == origin.Architecture) {
			return Release{
				OS:     os,
				Series: series,
			}, nil
		}
	}
	return Release{}, errors.NotFoundf("release")
}
