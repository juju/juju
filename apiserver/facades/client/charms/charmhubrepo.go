// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/juju/charm/v8"
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
	Info(ctx context.Context, name string, options ...charmhub.InfoOption) (transport.InfoResponse, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type chRepo struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel call the CharmHub version of
// ResolveWithPreferredChannel.
func (c *chRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Debugf("Resolving CharmHub charm %q", curl)

	if curl.Revision != -1 {
		return nil, params.CharmOrigin{}, nil, errors.Errorf("specifying a revision is not supported, please use a channel.")
	}

	refreshRes, err := c.refreshOne(curl, apicharm.APICharmOrigin(origin).CoreCharmOrigin())
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh")
	}

	if resErr := refreshRes.Error; resErr != nil {
		switch resErr.Code {
		case transport.ErrorCodeInvalidCharmPlatform:
			// We've got a invalid charm platform error, the error should contain
			// a valid platform to query again to get the right information. If
			// the platform is empty, consider it a failure.
			var platform transport.Platform
			for _, platform = range resErr.Extra.DefaultPlatforms {
				if platform.Architecture != origin.Architecture {
					continue
				}
				break
			}

			origin.OS = platform.OS
			origin.Series = platform.Series

		case transport.ErrorCodeRevisionNotFound:
			if len(resErr.Extra.Releases) == 0 {
				return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh error: %s", resErr.Message)
			}
			if origin.Series == "" {
				suggestions := composeSuggestions(resErr.Extra.Releases, origin)
				return nil, params.CharmOrigin{}, nil, errors.Errorf("no charm or bundle matching channel or platform; suggestions: %v", strings.Join(suggestions, ", "))
			}

			release, err := selectReleaseByArchAndChannel(resErr.Extra.Releases, origin)
			if err != nil {
				return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh error: %s", resErr.Message)
			}

			origin.OS = release.OS
			origin.Series = release.Series

		default:
			return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh error: %s", resErr.Message)
		}
		if origin.Series == "" {
			return nil, params.CharmOrigin{}, nil, errors.NotValidf("series for %s", curl.Name)
		}

		refreshRes, err = c.refreshOne(curl, apicharm.APICharmOrigin(origin).CoreCharmOrigin())
		if err != nil {
			return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "refresh retry")
		}
		if resErr := refreshRes.Error; resErr != nil {
			return nil, params.CharmOrigin{}, nil, errors.Errorf("refresh retry: %s", resErr.Message)
		}
	}

	// Use the channel that was actually picked by the API. This should
	// account for the closed tracks in a given channel.
	channel, err := corecharm.ParseChannelNormalize(refreshRes.EffectiveChannel)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "invalid channel")
	}

	var track *string
	if channel.Track != "" {
		track = &channel.Track
	}

	// Ensure we send the updated curl back, with all the correct segments.
	revision := refreshRes.Entity.Revision
	resCurl := curl.
		WithSeries(origin.Series).
		WithArchitecture(origin.Architecture).
		WithRevision(revision)

	resOrigin := origin
	// TODO (stickupkid): This is currently hardcoded as the API doesn't support
	// bundles.
	resOrigin.Type = "charm"
	resOrigin.ID = refreshRes.ID
	resOrigin.Hash = refreshRes.Entity.Download.HashSHA256
	resOrigin.Track = track
	resOrigin.Risk = string(channel.Risk)
	resOrigin.Revision = &revision

	logger.Criticalf("CURL %v ORIGIN %v", curl, resOrigin)

	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	return resCurl, outputOrigin, []string{outputOrigin.Series}, errors.Trace(err)
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
	logger.Criticalf("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return transport.RefreshResponse{}, errors.Errorf("more than 1 result found")
	}

	return result[0], nil
}

type Method string

const (
	MethodRevision Method = "revision"
	MethodChannel  Method = "channel"
	MethodID       Method = "id"
)

// refreshConfig creates a RefreshConfig for the given input.
// If the origin.ID is not set, a install refresh config is returned. For
//   install. Channel and Revision are mutually exclusive in the api, only
//   one will be used.  Channel first, Revision is a fallback.
// If the origin.ID is set, a refresh config is returned.
func refreshConfig(curl *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	var channel string
	if origin.Channel != nil && !origin.Channel.Empty() {
		channel = origin.Channel.String()
	} else {
		channel = corecharm.DefaultChannelString
	}

	// Work out the correct install method.
	var rev int
	var method Method
	if origin.Revision != nil && *origin.Revision >= 0 {
		rev = *origin.Revision
		method = MethodRevision
	}
	if method != MethodRevision && origin.ID == "" && channel != "" {
		method = MethodChannel
	}
	if method == MethodRevision && origin.ID != "" {
		method = MethodID
	}

	logger.Criticalf("Method %s %v %v %v", method, curl, spew.Sdump(origin), channel)

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
		// Install from just the name and the channel.
		cfg, err = charmhub.InstallOneFromChannel(curl.Name, channel, platform)
	case MethodRevision:
		// If there is a revision, install it using that.
		cfg, err = charmhub.InstallOneFromRevision(curl.Name, rev, platform)
	case MethodID:
		// If we have a ID, revision and channel, then use that.
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

func composeSuggestions(releases []transport.Release, origin params.CharmOrigin) []string {
	var suggestions []string
	for _, release := range releases {
		platform := release.Platform
		components := strings.Split(platform, "/")
		if len(components) != 3 {
			suggestions = append(suggestions, "channel %s", release.Channel)
			continue
		}

		arch, os, series := components[2], components[0], components[1]
		if arch == "all" {
			arch = origin.Architecture
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
		components := strings.Split(platform, "/")
		if len(components) != 3 {
			continue
		}

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

		arch, os, series := components[2], components[0], components[1]
		if (channel == nil || channel.String() == release.Channel) && (arch == "all" || arch == origin.Architecture) {
			return Release{
				OS:     os,
				Series: series,
			}, nil
		}
	}
	return Release{}, errors.NotFoundf("release")
}
