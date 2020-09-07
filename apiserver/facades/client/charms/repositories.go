// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"bytes"
	"context"
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/charmrepo/v6/csclient"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

// CharmHubClient represents the methods required of a
// client to install or upgrade a CharmHub charm.
type CharmHubClient interface {
	GetCharmFromURL(curl *url.URL, archivePath string) (*charm.CharmArchive, error)
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type chRepo struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel call the CharmHub version of
// ResolveWithPreferredChannel.
func (c *chRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmHub charm %q", curl)
	info, err := c.client.Info(context.TODO(), curl.Name)
	if err != nil {
		// Improve error message here
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	channel, err := makeChannel(origin)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	// If no revision nor channel specified, use the default release.
	if curl.Revision == -1 && channel.String() == "" {
		return c.resolveViaChannelMap(curl, origin, info.DefaultRelease)
	}

	channelMap, err := findChannelMap(curl.Revision, channel, info.ChannelMap)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}
	return c.resolveViaChannelMap(curl, origin, channelMap)
}

// GetCharm downloads the provided download URL from CharmHub using the provided
// archive path.
// A charm archive is returned.
func (c *chRepo) GetCharm(curl *charm.URL, resourceURL *url.URL, archivePath string) (*charm.CharmArchive, error) {
	logger.Tracef("GetCharm from CharmHub %q from %q", curl.String(), resourceURL.String())
	return c.client.Download(context.TODO(), resourceURL, archivePath)
}

// FindDownloadURL returns the url from which to download the CharmHub
// charm defined by the provided curl and charm origin.  An updated
// charm origin is also returned with the ID and hash for the charm
// to be downloaded.  If the provided charm origin has no ID, it is
// assumed that the charm is being installed, not refreshed.
func (c *chRepo) FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	cfg, err := refreshConfig(curl, origin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return nil, corecharm.Origin{}, errors.Errorf("More than 1 result found")
	}
	findResult := result[0]
	logger.Criticalf("FindDownloadURL received %+v", findResult)
	if findResult.Error != nil {
		// TODO: (hml) 4-sep-2020
		// When list of error codes available, create real error for them.
		return nil, corecharm.Origin{}, errors.Errorf("%s: %s", findResult.Error.Code, findResult.Error.Message)
	}
	origin.ID = findResult.Entity.ID
	origin.Hash = findResult.Entity.Download.HashSHA265
	durl, err := url.Parse(findResult.Entity.Download.URL)
	return durl, origin, errors.Trace(err)
}

func refreshConfig(curl *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	var rev int
	if origin.Revision != nil {
		rev = *origin.Revision
	}
	var channel string
	if origin.Channel != nil {
		channel = origin.Channel.String()
	}
	var seriesOS string
	if curl.Series != "" {
		opSys, err := series.GetOSFromSeries(curl.Series)
		if err != nil {
			return nil, errors.Trace(err)
		}
		seriesOS = opSys.String()
	}
	var cfg charmhub.RefreshConfig
	var err error

	switch {
	case origin.ID == "" && channel != "":
		// If there is no origin ID, we haven't downloaded this charm before.
		// Try channel first.
		cfg, err = charmhub.InstallOneFromChannel(curl.Name, channel, seriesOS, curl.Series)
	case origin.ID == "" && channel == "":
		// If there is no origin ID, we haven't downloaded this charm before.
		// No channel, try with revision.
		cfg, err = charmhub.InstallOneFromRevision(curl.Name, rev, seriesOS, curl.Series)
	case origin.ID != "":
		// This must be a charm upgrade if we have an ID.  Use the refresh action
		// for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(origin.ID, rev, channel, seriesOS, curl.Series)
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
	if track == "" {
		// If Risk only, assume "latest"
		track = "latest"
	}
	return corecharm.MakeChannel(track, origin.Risk, "")
}

func findChannelMap(rev int, preferredChannel corecharm.Channel, channelMaps []transport.ChannelMap) (transport.ChannelMap, error) {
	if len(channelMaps) == 0 {
		return transport.ChannelMap{}, errors.NotValidf("no channels provided by CharmHub")
	}
	switch {
	case preferredChannel.String() != "" && rev != -1:
		return findByRevisionAndChannel(rev, preferredChannel, channelMaps)
	case preferredChannel.String() != "":
		return findByChannel(preferredChannel, channelMaps)
	default: // rev != -1
		return findByRevision(rev, channelMaps)
	}
}

func findByRevision(rev int, channelMaps []transport.ChannelMap) (transport.ChannelMap, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev {
			// Channel map is in order of most newest/stable channel,
			// return the first of the requested revision.
			return cMap, nil
		}
	}
	return transport.ChannelMap{}, errors.NotFoundf("charm revision %d", rev)
}

func findByChannel(preferredChannel corecharm.Channel, channelMaps []transport.ChannelMap) (transport.ChannelMap, error) {
	for _, cMap := range channelMaps {
		if matchChannel(preferredChannel, cMap.Channel) {
			return cMap, nil
		}
	}
	return transport.ChannelMap{}, errors.NotFoundf("channel %q", preferredChannel.String())
}

func findByRevisionAndChannel(rev int, preferredChannel corecharm.Channel, channelMaps []transport.ChannelMap) (transport.ChannelMap, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev && matchChannel(preferredChannel, cMap.Channel) {
			return cMap, nil
		}
	}
	return transport.ChannelMap{}, errors.NotFoundf("charm revision %d for channel %q", rev, preferredChannel.String())
}

func matchChannel(one corecharm.Channel, two transport.Channel) bool {
	return one.String() == two.Name
}

func (c *chRepo) resolveViaChannelMap(curl *charm.URL, origin params.CharmOrigin, channelMap transport.ChannelMap) (*charm.URL, params.CharmOrigin, []string, error) {
	mapChannel := channelMap.Channel
	mapRevision := channelMap.Revision

	curl.Revision = mapRevision.Revision
	origin.Revision = &mapRevision.Revision
	origin.Risk = mapChannel.Risk
	origin.Track = &mapChannel.Track

	meta, err := unmarshalCharmMetadata(mapRevision.MetadataYAML)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Annotatef(err, "cannot unmarshal charm metadata")
	}
	return curl, origin, meta.Series, nil
}

func unmarshalCharmMetadata(metadataYAML string) (*charm.Meta, error) {
	if metadataYAML == "" {
		return nil, nil
	}
	m := metadataYAML
	meta, err := charm.ReadMeta(bytes.NewBufferString(m))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return meta, nil
}

type csRepo struct {
	repo CSRepository
}

// ResolveWithPreferredChannel calls the CharmStore version of
// ResolveWithPreferredChannel.  Convert CharmStore channel to
// and from the charm Origin.
func (c *csRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmStore charm %q with channel %q", curl, origin.Risk)
	// A charm origin risk is equivalent to a charm store channel
	newCurl, newRisk, supportedSeries, err := c.repo.ResolveWithPreferredChannel(curl, csparams.Channel(origin.Risk))
	newOrigin := origin
	newOrigin.Risk = string(newRisk)
	return newCurl, newOrigin, supportedSeries, err
}

func (c *csRepo) GetCharm(curl *charm.URL, _ *url.URL, archivePath string) (*charm.CharmArchive, error) {
	logger.Tracef("CharmStore GetCharm %q", curl)
	return c.repo.Get(curl, archivePath)
}

func (c *csRepo) FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	logger.Tracef("CharmStore FindDownloadURL %q", curl)
	return nil, origin, nil
}

type CSResolverGetterFunc func(args ResolverGetterParams) (CSRepository, error)

type ResolverGetterParams struct {
	CSURL              string
	Channel            string
	CharmStoreMacaroon *macaroon.Macaroon
}

// CSRepository is the part of charmrepo.Charmstore that we need to
// resolve a charm url, install or upgrade a charm store charm.
type CSRepository interface {
	Get(curl *charm.URL, archivePath string) (*charm.CharmArchive, error)
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}

func csResolverGetter(args ResolverGetterParams) (CSRepository, error) {
	csClient, err := openCSClient(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	repo := charmrepo.NewCharmStoreFromClient(csClient)
	return repo, nil
}

func openCSClient(args ResolverGetterParams) (*csclient.Client, error) {
	csURL, err := url.Parse(args.CSURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	csParams := csclient.Params{
		URL:          csURL.String(),
		BakeryClient: httpbakery.NewClient(),
	}

	if args.CharmStoreMacaroon != nil {
		// Set the provided charmstore authorizing macaroon
		// as a cookie in the HTTP client.
		// TODO(cmars) discharge any third party caveats in the macaroon.
		ms := []*macaroon.Macaroon{args.CharmStoreMacaroon}
		if err := httpbakery.SetCookie(csParams.BakeryClient.Jar, csURL, charmstore.MacaroonNamespace, ms); err != nil {
			return nil, errors.Trace(err)
		}
	}
	csClient := csclient.New(csParams)
	channel := csparams.Channel(args.Channel)
	if channel != csparams.NoChannel {
		csClient = csClient.WithChannel(channel)
	}
	return csClient, nil
}
