// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	csclient "github.com/juju/charmrepo/v6/csclient"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmstore"
)

type chResolver struct {
	client *charmhub.Client
}

// ResolveWithPreferredChannel call the CharmHub version of
// ResolveWithPreferredChannel.
func (c *chResolver) ResolveWithPreferredChannel(*charm.URL, params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {

	return nil, params.CharmOrigin{}, nil, nil
}

type csResolver struct {
	resolver CSURLResolver
}

// ResolveWithPreferredChannel calls the CharmStore version of
// ResolveWithPreferredChannel.  Convert CharmStore channel to
// and from the charm Origin.
func (c *csResolver) ResolveWithPreferredChannel(curl *charm.URL, origin params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error) {
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
		return nil, err
	}
	repo := charmrepo.NewCharmStoreFromClient(csClient)
	return repo, nil
}

func openCSClient(args ResolverGetterParams) (*csclient.Client, error) {
	csURL, err := url.Parse(args.CSURL)
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}
	csClient := csclient.New(csParams)
	channel := csparams.Channel(args.Channel)
	if channel != csparams.NoChannel {
		csClient = csClient.WithChannel(channel)
	}
	return csClient, nil
}
