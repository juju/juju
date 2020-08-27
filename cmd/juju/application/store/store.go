// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(natefinch): change the code in this file to use the
// github.com/juju/juju/charmstore package to interact with the charmstore.

package store

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
	"github.com/juju/juju/cmd/juju/common"
)

// AddCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state. For non-public charm URLs, this function also
// handles the macaroon authorization process using the given csClient.
// The resulting charm URL of the added charm is displayed on stdout.
func AddCharmFromURL(client CharmAdder, cs MacaroonGetter, curl *charm.URL, channel csparams.Channel, force bool) (*charm.URL, *macaroon.Macaroon, error) {
	var csMac *macaroon.Macaroon
	if err := client.AddCharm(curl, channel, force); err != nil {
		if !params.IsCodeUnauthorized(err) {
			return nil, nil, errors.Trace(err)
		}
		m, err := authorizeCharmStoreEntity(cs, curl)
		if err != nil {
			return nil, nil, common.MaybeTermsAgreementError(err)
		}
		if err := client.AddCharmWithAuthorization(curl, channel, m, force); err != nil {
			return nil, nil, errors.Trace(err)
		}
		csMac = m
	}
	return curl, csMac, nil
}

// NewCharmStoreClient is called to obtain a charm store client.
// It is defined as a variable so it can be changed for testing purposes.
var NewCharmStoreClient = func(client *httpbakery.Client, csURL string) *csclient.Client {
	return csclient.New(csclient.Params{
		URL:          csURL,
		BakeryClient: client,
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
