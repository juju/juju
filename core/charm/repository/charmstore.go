// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"io"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/charmrepo/v7"
	"github.com/juju/charmrepo/v7/csclient"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/version"
)

// CharmStoreClient describes the API exposed by the charmstore client.
type CharmStoreClient interface {
	Get(charmURL *charm.URL, archivePath string) (*charm.CharmArchive, error)
	ResolveWithPreferredChannel(charmURL *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error)
	Meta(*charm.URL, interface{}) (*charm.URL, error)
	GetFileFromArchive(charmURL *charm.URL, filename string) (io.ReadCloser, error)
}

// CharmStoreRepository provides an API for charm-related operations using charmstore.
type CharmStoreRepository struct {
	logger        Logger
	charmstoreURL string
	clientFactory func(storeURL string, channel csparams.Channel, macaroons macaroon.Slice) (CharmStoreClient, error)
}

// csMetadataResponse encodes a metadata lookup response from the charmstore API.
// The client uses reflection to extract the field names from this struct and
// pass them to the charmstore API. The field names must therefore not change
// or the metadata request will fail.
type csMetadataResponse struct {
	CharmMetadata *charm.Meta
	CharmConfig   *charm.Config
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

// GetEssentialMetadata resolves each provided MetadataRequest and returns back
// a slice with the results. The results include the minimum set of metadata
// that is required for deploying each charm.
func (c *CharmStoreRepository) GetEssentialMetadata(reqs ...corecharm.MetadataRequest) ([]corecharm.EssentialMetadata, error) {
	var res = make([]corecharm.EssentialMetadata, len(reqs))

	for reqIdx, req := range reqs {
		// NOTE(achilleas): due to the way that the charmstore client
		// was originally implemented we unfortunately need to create a
		// new client per request.
		channel := csparams.Channel(req.Origin.Channel.Risk)
		client, err := c.clientFactory(c.charmstoreURL, channel, req.Macaroons)
		if err != nil {
			return nil, errors.Annotatef(err, "obain charmstore client for %q", req.CharmURL)
		}

		var csMetaRes csMetadataResponse
		if _, err = client.Meta(req.CharmURL, &csMetaRes); err != nil {
			return nil, errors.Annotatef(err, "retrieving metadata for %q", req.CharmURL)
		}

		res[reqIdx].Meta = csMetaRes.CharmMetadata
		res[reqIdx].Config = csMetaRes.CharmConfig
		res[reqIdx].ResolvedOrigin = req.Origin
	}

	return res, nil
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
