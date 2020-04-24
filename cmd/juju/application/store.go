// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(natefinch): change the code in this file to use the
// github.com/juju/juju/charmstore package to interact with the charmstore.

package application

import (
	"net/url"

	"github.com/juju/charm/v7"
	"github.com/juju/charmrepo/v5/csclient"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
)

// SeriesConfig defines the single config method that we need to resolve
// changes.
type SeriesConfig interface {
	// DefaultSeries returns the configured default Ubuntu series for the environment,
	// and whether the default series was explicitly configured on the environment.
	DefaultSeries() (string, bool)
}

// ResolveCharmFunc is the type of a function that resolves a charm URL with
// an optionally specified preferred channel.
type ResolveCharmFunc func(
	resolveWithChannel func(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
	preferredChannel csparams.Channel,
) (*charm.URL, csparams.Channel, []string, error)

func resolveCharm(
	resolveWithChannel func(*charm.URL, csparams.Channel) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
	preferredChannel csparams.Channel,
) (*charm.URL, csparams.Channel, []string, error) {
	if url.Schema != "cs" {
		return nil, csparams.NoChannel, nil, errors.Errorf("unknown schema for charm URL %q", url)
	}

	resultURL, channel, supportedSeries, err := resolveWithChannel(url, preferredChannel)
	if err != nil {
		return nil, csparams.NoChannel, nil, errors.Trace(err)
	}
	if resultURL.Series != "" && len(supportedSeries) == 0 {
		supportedSeries = []string{resultURL.Series}
	}
	return resultURL, channel, supportedSeries, nil
}

// TODO(ericsnow) Return charmstore.CharmID from addCharmFromURL()?

// addCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state. For non-public charm URLs, this function also
// handles the macaroon authorization process using the given csClient.
// The resulting charm URL of the added charm is displayed on stdout.
func addCharmFromURL(client CharmAdder, cs macaroonGetter, curl *charm.URL, channel csparams.Channel, force bool) (*charm.URL, *macaroon.Macaroon, error) {
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

// newCharmStoreClient is called to obtain a charm store client.
// It is defined as a variable so it can be changed for testing purposes.
var newCharmStoreClient = func(client *httpbakery.Client, csURL string) *csclient.Client {
	return csclient.New(csclient.Params{
		URL:          csURL,
		BakeryClient: client,
	})
}

type macaroonGetter interface {
	Get(endpoint string, extra interface{}) error
}

// authorizeCharmStoreEntity acquires and return the charm store delegatable macaroon to be
// used to add the charm corresponding to the given URL.
// The macaroon is properly attenuated so that it can only be used to deploy
// the given charm URL.
func authorizeCharmStoreEntity(csClient macaroonGetter, curl *charm.URL) (*macaroon.Macaroon, error) {
	endpoint := "/delegatable-macaroon?id=" + url.QueryEscape(curl.String())
	var m *macaroon.Macaroon
	if err := csClient.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
