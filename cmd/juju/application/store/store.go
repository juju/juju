// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v9"
	"github.com/juju/charmrepo/v7"
	"github.com/juju/charmrepo/v7/csclient"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/version"
)

// AddCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state.
func AddCharmFromURL(client CharmAdder, curl *charm.URL, origin commoncharm.Origin, force bool) (*charm.URL, commoncharm.Origin, error) {
	resultOrigin, err := client.AddCharm(curl, origin, force)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			return nil, commoncharm.Origin{}, errors.Forbiddenf(err.Error())
		}
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	return curl, resultOrigin, nil
}

// AddCharmWithAuthorizationFromURL calls the appropriate client API calls to
// add the given charm URL to state. For non-public charm URLs, this function
// also handles the macaroon authorization process using the given csClient.
// The resulting charm URL of the added charm is displayed on stdout.
func AddCharmWithAuthorizationFromURL(client CharmAdder, curl *charm.URL, origin commoncharm.Origin, force bool) (*charm.URL, commoncharm.Origin, error) {
	resultOrigin, err := client.AddCharm(curl, origin, force)
	if err != nil {
		if !params.IsCodeUnauthorized(err) {
			return nil, commoncharm.Origin{}, errors.Trace(err)
		}
		if resultOrigin, err = client.AddCharm(curl, origin, force); err != nil {
			return nil, commoncharm.Origin{}, errors.Trace(err)
		}
	}
	return curl, resultOrigin, nil
}

// NewCharmStoreClient is called to obtain a charm store client.
// It is defined as a variable so it can be changed for testing purposes.
var NewCharmStoreClient = func(client *httpbakery.Client, csURL string) *csclient.Client {
	return csclient.New(csclient.Params{
		URL:            csURL,
		BakeryClient:   client,
		UserAgentValue: version.UserAgentVersion,
	})
}

// NewCharmStoreAdaptor combines charm store functionality with the ability to get a macaroon.
func NewCharmStoreAdaptor(client *httpbakery.Client, csURL string) *CharmStoreAdaptor {
	cstoreClient := NewCharmStoreClient(client, csURL)
	return &CharmStoreAdaptor{
		CharmrepoForDeploy: charmrepo.NewCharmStoreFromClient(cstoreClient),
		MacaroonGetter:     cstoreClient,
	}
}

type CharmStoreAdaptor struct {
	CharmrepoForDeploy
	MacaroonGetter
}
