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
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

type CharmHubClient interface {
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
}

type chResolver struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel call the CharmHub version of
// ResolveWithPreferredChannel.
func (c *chResolver) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
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

func (c *chResolver) resolveViaChannelMap(curl *charm.URL, origin params.CharmOrigin, channelMap transport.ChannelMap) (*charm.URL, params.CharmOrigin, []string, error) {
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

type csResolver struct {
	resolver CSURLResolver
}

// ResolveWithPreferredChannel calls the CharmStore version of
// ResolveWithPreferredChannel.  Convert CharmStore channel to
// and from the charm Origin.
func (c *csResolver) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
	logger.Tracef("Resolving CharmStore charm %q with channel %q", curl, origin.Risk)
	// A charm origin risk is equivalent to a charm store channel
	newCurl, newRisk, supportedSeries, err := c.resolver.ResolveWithPreferredChannel(curl, csparams.Channel(origin.Risk))
	newOrigin := origin
	newOrigin.Risk = string(newRisk)
	return newCurl, newOrigin, supportedSeries, err
}

type CSResolverGetterFunc func(args ResolverGetterParams) (CSURLResolver, error)

type ResolverGetterParams struct {
	CSURL              string
	Channel            string
	CharmStoreMacaroon *macaroon.Macaroon
}

// CSURLResolver is the part of charmrepo.Charmstore that we need to
// resolve a charm url.
type CSURLResolver interface {
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}

func csResolverGetter(args ResolverGetterParams) (CSURLResolver, error) {
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
