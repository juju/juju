// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v9"
	"github.com/juju/charmrepo/v7"
	"github.com/juju/charmrepo/v7/csclient"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
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
func AddCharmWithAuthorizationFromURL(client CharmAdder, cs MacaroonGetter, curl *charm.URL, origin commoncharm.Origin, force bool) (*charm.URL, *macaroon.Macaroon, commoncharm.Origin, error) {
	var csMac *macaroon.Macaroon
	resultOrigin, err := client.AddCharm(curl, origin, force)
	if err != nil {
		if !params.IsCodeUnauthorized(err) {
			return nil, nil, commoncharm.Origin{}, errors.Trace(err)
		}
		m, err := authorizeCharmStoreEntity(cs, curl)
		if err != nil {
			return nil, nil, commoncharm.Origin{}, common.MaybeTermsAgreementError(err)
		}
		if resultOrigin, err = client.AddCharmWithAuthorization(curl, origin, m, force); err != nil {
			return nil, nil, commoncharm.Origin{}, errors.Trace(err)
		}
		csMac = m
	}
	return curl, csMac, resultOrigin, nil
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

// authorizeCharmStoreEntity acquires and return the charm store
// delegatable macaroon to be used to add the charm corresponding
// to the given URL. The macaroon is properly attenuated so that
// it can only be used to deploy the given charm URL.
func authorizeCharmStoreEntity(csClient MacaroonGetter, curl *charm.URL) (*macaroon.Macaroon, error) {
	endpoint := "/delegatable-macaroon?id=" + url.QueryEscape(curl.String())
	var m *macaroon.Macaroon
	if err := csClient.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
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
