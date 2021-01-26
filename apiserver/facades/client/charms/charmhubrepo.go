// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"net/url"

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
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	var (
		series   []string
		resErr   = refreshRes.Error
		revision = -1
		hash     string
	)
	if resErr != nil {
		if resErr.Code != transport.ErrorCodeInvalidCharmPlatform {
			return nil, params.CharmOrigin{}, nil, errors.Trace(err)
		}

		// We located the charm, but unfortunately the platform didn't match
		// so in this case, we return a valid result, but the supported series
		// now contains the result from the error.
		for _, platform := range resErr.Extra.DefaultPlatforms {
			if platform.Architecture != origin.Architecture {
				continue
			}

			series = append(series, platform.Series)
		}
	} else {
		series = append(series, origin.Series)
		revision = refreshRes.Entity.Revision
		hash = refreshRes.Entity.Download.HashSHA256
	}

	// Ensure we send the updated curl back, with all the correct segments.
	resCurl := curl.
		WithArchitecture(origin.Architecture).
		WithRevision(revision)

	resOrigin := origin
	// TODO (stickupkid): This is currently hardcoded as the API doesn't support
	// bundles.
	resOrigin.Type = "charm"
	resOrigin.ID = refreshRes.ID
	resOrigin.Hash = hash

	if len(series) > 0 {
		resCurl = resCurl.WithSeries(series[0])
		resOrigin.Series = series[0]
	}
	if revision >= 0 {
		resOrigin.Revision = &revision
	}

	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	return resCurl, outputOrigin, series, errors.Trace(err)
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
	logger.Debugf("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return transport.RefreshResponse{}, errors.Errorf("more than 1 result found")
	}

	return result[0], nil
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
