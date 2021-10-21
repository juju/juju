// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/charmrepo/v6/csclient"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/version"
)

// CharmStoreClient describes the API exposed by the charmstore client.
type CharmStoreClient interface {
	Get(charmURL *charm.URL, archivePath string) (*charm.CharmArchive, error)
	ResolveWithPreferredChannel(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
}

// CharmStoreRepository provides an API for charm-related operations using charmstore.
type CharmStoreRepository struct {
	logger        Logger
	charmstoreURL string
	clientFactory func(storeURL string, channel csparams.Channel, macaroons macaroon.Slice) (CharmStoreClient, error)
}

// NewCharmStoreRepository returns a new repository instance using the provided
// charmstore client.
func NewCharmStoreRepository(logger Logger, charmstoreURL string) *CharmStoreRepository {
	return &CharmStoreRepository{
		logger:        logger,
		charmstoreURL: charmstoreURL,
		clientFactory: makeCharmStoreClient,
	}
}

// ResolveWithPreferredChannel queries the store and resolves the provided
// charm URL/origin tuple to a charm URL that corresponds to a downloadable
// charm/bundle from the store.
func (c *CharmStoreRepository) ResolveWithPreferredChannel(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice) (*charm.URL, corecharm.Origin, []string, error) {
	channel := csparams.Channel(requestedOrigin.Channel.Risk)
	client, err := c.clientFactory(c.charmstoreURL, channel, macaroons)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Trace(err)
	}

	return c.doResolveWithClient(client, charmURL, requestedOrigin)
}

func (c *CharmStoreRepository) doResolveWithClient(client CharmStoreClient, charmURL *charm.URL, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []string, error) {
	channel := csparams.Channel(requestedOrigin.Channel.Risk)
	c.logger.Tracef("Resolving CharmStore charm %q with channel %q", charmURL, channel)

	newCurl, newRisk, supportedSeries, err := client.ResolveWithPreferredChannel(charmURL, channel)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Trace(err)
	}

	var t string
	switch newCurl.Series {
	case "bundle":
		t = "bundle"
	default:
		t = "charm"
	}

	newOrigin := requestedOrigin
	newOrigin.Type = t
	newOrigin.Channel.Risk = charm.Risk(newRisk)
	return newCurl, newOrigin, supportedSeries, err
}

// DownloadCharm retrieves specified charm from the store and saves its
// contents to the specified path.
func (c *CharmStoreRepository) DownloadCharm(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error) {
	channel := csparams.Channel(requestedOrigin.Channel.Risk)
	client, err := c.clientFactory(c.charmstoreURL, channel, macaroons)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	// Resolve the URL first to get the correct channel
	resolvedCharmURL, resolvedOrigin, _, err := c.doResolveWithClient(client, charmURL, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	c.logger.Tracef("DownloadCharm %q", resolvedCharmURL)
	charmArchive, err := client.Get(resolvedCharmURL, archivePath)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	return charmArchive, resolvedOrigin, nil
}

// GetDownloadURL resolves the specified charm/bundle URL into a url.URL which
// can be used to download the blob associated with the charm/bundle.
func (c *CharmStoreRepository) GetDownloadURL(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice) (*url.URL, corecharm.Origin, error) {
	channel := csparams.Channel(requestedOrigin.Channel.Risk)
	client, err := c.clientFactory(c.charmstoreURL, channel, macaroons)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	// Resolve the URL first to get the correct channel
	resolvedCharmURL, resolvedOrigin, _, err := c.doResolveWithClient(client, charmURL, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	c.logger.Tracef("GetDownloadURL %q", resolvedCharmURL)

	curl, err := url.Parse(resolvedCharmURL.String())
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	return curl, resolvedOrigin, nil
}

// ListResources returns the resources for a given charm and origin. This is
// a no-op for charmstore.
func (c *CharmStoreRepository) ListResources(charmURL *charm.URL, _ corecharm.Origin, _ macaroon.Slice) ([]charmresource.Resource, error) {
	c.logger.Tracef("ListResources %q", charmURL)
	return nil, nil
}

func makeCharmStoreClient(charmstoreURL string, defaultChannel csparams.Channel, macaroons macaroon.Slice) (CharmStoreClient, error) {
	apiURL, err := url.Parse(charmstoreURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	csParams := csclient.Params{
		URL:            apiURL.String(),
		BakeryClient:   httpbakery.NewClient(),
		UserAgentValue: version.UserAgentVersion,
	}

	if len(macaroons) != 0 {
		if err := httpbakery.SetCookie(csParams.BakeryClient.Jar, apiURL, charmstore.MacaroonNamespace, macaroons); err != nil {
			return nil, errors.Trace(err)
		}
	}

	csClient := csclient.New(csParams)
	if defaultChannel != csparams.NoChannel {
		csClient = csClient.WithChannel(defaultChannel)
	}

	return charmrepo.NewCharmStoreFromClient(csClient), nil
}
