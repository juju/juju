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
	"github.com/juju/os/v2/series"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/selector"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

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
		resURL, resOrigin, serie, err := c.resolveViaChannelMap(info.Type, curl, origin, info.DefaultRelease)
		if err != nil {
			return nil, params.CharmOrigin{}, nil, errors.Trace(err)
		}

		resOrigin.ID = info.ID
		return resURL, resOrigin, serie, nil
	}

	logger.Debugf("Resolving charm with revision %d and/or channel %s", curl.Revision, channel.String())

	channelMap, err := findChannelMap(curl.Revision, channel, info.ChannelMap)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}
	resURL, resOrigin, serie, err := c.resolveViaChannelMap(info.Type, curl, origin, channelMap)
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	resOrigin.ID = info.ID
	return resURL, resOrigin, serie, nil
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
		return c.findBundleDownloadURL(curl, origin)
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

	origin.ID = findResult.Entity.ID
	origin.Hash = findResult.Entity.Download.HashSHA256

	durl, err := url.Parse(findResult.Entity.Download.URL)
	return durl, origin, errors.Trace(err)
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
			// TODO (stickupkid): FIX ME, charmhub ignores architecture
			// "sometimes"...
			// Architecture: origin.Platform.Architecture,
			Architecture: "all",
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

func findChannelMap(rev int, preferredChannel corecharm.Channel, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, error) {
	if len(channelMaps) == 0 {
		return transport.InfoChannelMap{}, errors.NotValidf("no channels provided by CharmHub")
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

func findByRevision(rev int, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev {
			// Channel map is in order of most newest/stable channel,
			// return the first of the requested revision.
			return cMap, nil
		}
	}
	return transport.InfoChannelMap{}, errors.NotFoundf("charm revision %d", rev)
}

func findByChannel(preferredChannel corecharm.Channel, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, error) {
	for _, cMap := range channelMaps {
		if matchChannel(preferredChannel, cMap.Channel) {
			return cMap, nil
		}
	}
	return transport.InfoChannelMap{}, errors.NotFoundf("channel %q", preferredChannel.String())
}

func findByRevisionAndChannel(rev int, preferredChannel corecharm.Channel, channelMaps []transport.InfoChannelMap) (transport.InfoChannelMap, error) {
	for _, cMap := range channelMaps {
		if cMap.Revision.Revision == rev && matchChannel(preferredChannel, cMap.Channel) {
			return cMap, nil
		}
	}
	return transport.InfoChannelMap{}, errors.NotFoundf("charm revision %d for channel %q", rev, preferredChannel.String())
}

func matchChannel(one corecharm.Channel, two transport.Channel) bool {
	return one.Normalize().String() == two.Name
}

func (c *chRepo) resolveViaChannelMap(t transport.Type, curl *charm.URL, origin params.CharmOrigin, channelMap transport.InfoChannelMap) (*charm.URL, params.CharmOrigin, []string, error) {
	mapChannel := channelMap.Channel
	mapRevision := channelMap.Revision

	curl.Revision = mapRevision.Revision

	origin.Type = t
	origin.Revision = &mapRevision.Revision
	origin.Risk = mapChannel.Risk
	origin.Track = &mapChannel.Track

	origin.Architecture = mapChannel.Platform.Architecture
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

// Metadata represents the return type for both charm types (charm and bundles)
type Metadata interface {
	ComputedSeries() []string
}

func unmarshalCharmMetadata(metadataYAML string) (Metadata, error) {
	meta, err := charm.ReadMeta(bytes.NewBufferString(metadataYAML))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return meta, nil
}

func unmarshalBundleMetadata(bundleYAML string) (Metadata, error) {
	meta, err := charm.ReadBundleData(bytes.NewBufferString(bundleYAML))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bundleMetadata{BundleData: meta}, nil
}

type bundleMetadata struct {
	*charm.BundleData
}

func (b bundleMetadata) ComputedSeries() []string {
	return []string{b.BundleData.Series}
}

type csRepo struct {
	repo CSRepository
}

// ResolveWithPreferredChannel calls the CharmStore version of
// ResolveWithPreferredChannel. Convert CharmStore channel to
// and from the charm Origin.
func (c *csRepo) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmStore charm %q with channel %q", curl, origin.Risk)
	// A charm origin risk is equivalent to a charm store channel
	newCurl, newRisk, supportedSeries, err := c.repo.ResolveWithPreferredChannel(curl, csparams.Channel(origin.Risk))
	if err != nil {
		return nil, params.CharmOrigin{}, nil, errors.Trace(err)
	}

	var t string
	switch newCurl.Series {
	case "bundle":
		t = "bundle"
	default:
		t = "charm"
	}

	newOrigin := origin
	newOrigin.Type = t
	newOrigin.Risk = string(newRisk)
	return newCurl, newOrigin, supportedSeries, err
}

func (c *csRepo) DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error) {
	logger.Tracef("CharmStore DownloadCharm %q", resourceURL)
	curl, err := charm.ParseURL(resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
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
