// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(natefinch): change the code in this file to use the
// github.com/juju/juju/charmstore package to interact with the charmstore.

package application

import (
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs/config"
)

func isSeriesSupported(requestedSeries string, supportedSeries []string) bool {
	for _, series := range supportedSeries {
		if series == requestedSeries {
			return true
		}
	}
	return false
}

// TODO(ericsnow) Return charmstore.CharmID from resolve()?

// ResolveCharmFunc is the type of a function that resolves a charm URL.
type ResolveCharmFunc func(
	resolveWithChannel func(*charm.URL) (*charm.URL, csparams.Channel, []string, error),
	conf *config.Config,
	url *charm.URL,
) (*charm.URL, csparams.Channel, []string, error)

func resolveCharm(
	resolveWithChannel func(*charm.URL) (*charm.URL, csparams.Channel, []string, error),
	conf *config.Config,
	url *charm.URL,
) (*charm.URL, csparams.Channel, []string, error) {
	if url.Schema != "cs" {
		return nil, csparams.NoChannel, nil, errors.Errorf("unknown schema for charm URL %q", url)
	}
	// If the user hasn't explicitly asked for a particular series,
	// query for the charm that matches the model's default series.
	// If this fails, we'll fall back to asking for whatever charm is available.
	defaultedSeries := false
	if url.Series == "" {
		if s, ok := conf.DefaultSeries(); ok {
			defaultedSeries = true
			// TODO(katco): Don't update the value passed in. Not only
			// is there no indication that this method will do so, we
			// return a charm.URL which signals to the developer that
			// we don't modify the original.
			url.Series = s
		}
	}

	resultURL, channel, supportedSeries, err := resolveWithChannel(url)
	if defaultedSeries && errors.Cause(err) == csparams.ErrNotFound {
		// we tried to use the model's default the series, but the store said it doesn't exist.
		// retry without the defaulted series, to take what we can get.
		url.Series = ""
		resultURL, channel, supportedSeries, err = resolveWithChannel(url)
	}
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
func addCharmFromURL(client CharmAdder, curl *charm.URL, channel csparams.Channel) (*charm.URL, *macaroon.Macaroon, error) {
	var csMac *macaroon.Macaroon
	if err := client.AddCharm(curl, channel); err != nil {
		if !params.IsCodeUnauthorized(err) {
			return nil, nil, errors.Trace(err)
		}
		m, err := client.AuthorizeCharmstoreEntity(curl)
		if err != nil {
			return nil, nil, common.MaybeTermsAgreementError(err)
		}
		if err := client.AddCharmWithAuthorization(curl, channel, m); err != nil {
			return nil, nil, errors.Trace(err)
		}
		csMac = m
	}
	return curl, csMac, nil
}

// newCharmStoreClient is called to obtain a charm store client.
// It is defined as a variable so it can be changed for testing purposes.
var newCharmStoreClient = func(client *httpbakery.Client) *csclient.Client {
	return csclient.New(csclient.Params{
		BakeryClient: client,
	})
}

// authorizeCharmStoreEntity acquires and return the charm store delegatable macaroon to be
// used to add the charm corresponding to the given URL.
// The macaroon is properly attenuated so that it can only be used to deploy
// the given charm URL.
func authorizeCharmStoreEntity(csClient *csclient.Client, curl *charm.URL) (*macaroon.Macaroon, error) {
	endpoint := "/delegatable-macaroon?id=" + url.QueryEscape(curl.String())
	var m *macaroon.Macaroon
	if err := csClient.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
