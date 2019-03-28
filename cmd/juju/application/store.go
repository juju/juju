// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(natefinch): change the code in this file to use the
// github.com/juju/juju/charmstore package to interact with the charmstore.

package application

import (
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/testcharms"
)

// SeriesConfig defines the single config method that we need to resolve
// changes.
type SeriesConfig interface {
	// DefaultSeries returns the configured default Ubuntu series for the environment,
	// and whether the default series was explicitly configured on the environment.
	DefaultSeries() (string, bool)
}

// ResolveCharmFunc is the type of a function that resolves a charm URL.
type ResolveCharmFunc func(
	resolveWithChannel func(*charm.URL) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
) (*charm.URL, csparams.Channel, []string, error)

func resolveCharm(
	resolveWithChannel func(*charm.URL) (*charm.URL, csparams.Channel, []string, error),
	url *charm.URL,
) (*charm.URL, csparams.Channel, []string, error) {
	if url.Schema != "cs" {
		return nil, csparams.NoChannel, nil, errors.Errorf("unknown schema for charm URL %q", url)
	}
	resultURL, channel, supportedSeries, err := resolveWithChannel(url)
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
func addCharmFromURL(client CharmAdder, curl *charm.URL, channel csparams.Channel, force bool) (*charm.URL, *macaroon.Macaroon, error) {
	var csMac *macaroon.Macaroon
	if err := client.AddCharm(curl, channel, force); err != nil {
		if !params.IsCodeUnauthorized(err) {
			return nil, nil, errors.Trace(err)
		}
		m, err := client.AuthorizeCharmstoreEntity(curl)
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

type charmstoreCommunicator interface {
	Get(endpoint string, extra interface{}) error
	WithChannel(csparams.Channel) charmstoreCommunicator // *csclient.Client
	// Latest(ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error)
	// ListResources(id *charm.URL) ([]params.Resource, error)
	// GetResource(id *charm.URL, name string, revision int) (csclient.ResourceData, error)
	// ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error)
}

type testingCharmstoreCommunicatorShim struct {
	testcharms.Charmstore
}

func (c *testingCharmstoreCommunicatorShim) WithChannel(channel csparams.Channel) charmstoreCommunicator {
	return c.WithChannel(channel)
}

type charmstoreCommunicatorShim struct {
	*csclient.Client
}

func (c *charmstoreCommunicatorShim) WithChannel(channel csparams.Channel) charmstoreCommunicator {
	return c.WithChannel(channel)
}

// newCharmStoreClient is called to obtain a charm store client.
// It is defined as a variable so it can be changed for testing purposes.
var newCharmStoreClient = func(client *httpbakery.Client, csURL string) *csclient.Client {
	return csclient.New(csclient.Params{
		URL:          csURL,
		BakeryClient: client,
	})
}

// authorizeCharmStoreEntity acquires and return the charm store delegatable macaroon to be
// used to add the charm corresponding to the given URL.
// The macaroon is properly attenuated so that it can only be used to deploy
// the given charm URL.
func authorizeCharmStoreEntity(csClient charmstoreCommunicator, curl *charm.URL) (*macaroon.Macaroon, error) {
	endpoint := "/delegatable-macaroon?id=" + url.QueryEscape(curl.String())
	var m *macaroon.Macaroon
	if err := csClient.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
