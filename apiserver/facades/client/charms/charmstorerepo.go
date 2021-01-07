// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/charmrepo/v6/csclient"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

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
