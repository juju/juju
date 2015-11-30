// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.cmd.juju")

// resolveCharmStoreEntityURL resolves the given charm or bundle URL string
// by looking it up in the appropriate charm repository.
// If it is a charm store URL, the given csParams will
// be used to access the charm store repository.
// If it is a local charm or bundle URL, the local charm repository at
// the given repoPath will be used. The given configuration
// will be used to add any necessary attributes to the repo
// and to resolve the default series if possible.
//
// resolveCharmStoreEntityURL also returns the charm repository holding
// the charm or bundle.
func resolveCharmStoreEntityURL(urlStr string, csParams charmrepo.NewCharmStoreParams, repoPath string, conf *config.Config) (*charm.URL, charmrepo.Interface, error) {
	ref, err := charm.ParseURL(urlStr)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	repo, err := charmrepo.InferRepository(ref, csParams, repoPath)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	repo = config.SpecializeCharmRepo(repo, conf)
	if ref.Series == "" {
		if defaultSeries, ok := conf.DefaultSeries(); ok {
			ref.Series = defaultSeries
		}
	}
	if ref.Schema == "local" && ref.Series == "" {
		possibleURL := *ref
		possibleURL.Series = config.LatestLtsSeries()
		logger.Errorf("The series is not specified in the environment (default-series) or with the charm. Did you mean:\n\t%s", &possibleURL)
		return nil, nil, errors.Errorf("cannot resolve series for charm: %q", ref)
	}
	// TODO(wallyworld) - charm store does not yet support returning the
	// supported series for a charm.
	ref, _, err = repo.Resolve(ref)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if ref.Series == "" {
		return nil, nil, errors.New("resolved charm URL has no series")
	}
	return ref, repo, nil
}

// addCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state. For non-public charm URLs, this function also
// handles the macaroon authorization process using the given csClient.
// The resulting charm URL of the added charm is displayed on stdout.
func addCharmFromURL(client *api.Client, curl *charm.URL, repo charmrepo.Interface, csclient *csClient) (*charm.URL, error) {
	switch curl.Schema {
	case "local":
		ch, err := repo.Get(curl)
		if err != nil {
			return nil, err
		}
		stateCurl, err := client.AddLocalCharm(curl, ch)
		if err != nil {
			return nil, err
		}
		curl = stateCurl
	case "cs":
		if err := client.AddCharm(curl); err != nil {
			if !params.IsCodeUnauthorized(err) {
				return nil, errors.Mask(err)
			}
			m, err := csclient.authorize(curl)
			if err != nil {
				return nil, errors.Mask(err)
			}
			if err := client.AddCharmWithAuthorization(curl, m); err != nil {
				return nil, errors.Mask(err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported charm URL schema: %q", curl.Schema)
	}
	return curl, nil
}

// csClient gives access to the charm store server and provides parameters
// for connecting to the charm store.
type csClient struct {
	params charmrepo.NewCharmStoreParams
}

// newCharmStoreClient is called to obtain a charm store client
// including the parameters for connecting to the charm store, and
// helpers to save the local authorization cookies and to authorize
// non-public charm deployments. It is defined as a variable so it can
// be changed for testing purposes.
var newCharmStoreClient = func(client *http.Client) *csClient {
	return &csClient{
		params: charmrepo.NewCharmStoreParams{
			HTTPClient:   client,
			VisitWebPage: httpbakery.OpenWebBrowser,
		},
	}
}

// authorize acquires and return the charm store delegatable macaroon to be
// used to add the charm corresponding to the given URL.
// The macaroon is properly attenuated so that it can only be used to deploy
// the given charm URL.
func (c *csClient) authorize(curl *charm.URL) (*macaroon.Macaroon, error) {
	client := csclient.New(csclient.Params{
		URL:          c.params.URL,
		HTTPClient:   c.params.HTTPClient,
		VisitWebPage: c.params.VisitWebPage,
	})
	var m *macaroon.Macaroon
	if err := client.Get("/delegatable-macaroon", &m); err != nil {
		return nil, errors.Trace(err)
	}
	if err := m.AddFirstPartyCaveat("is-entity " + curl.String()); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
