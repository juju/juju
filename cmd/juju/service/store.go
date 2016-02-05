// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

// maybeTermsAgreementError returns err as a *termsAgreementError
// if it has a "terms agreement required" error code, otherwise
// it returns err unchanged.
func maybeTermsAgreementError(err error) error {
	const code = "term agreement required"
	e, ok := errors.Cause(err).(*httpbakery.DischargeError)
	if !ok || e.Reason == nil || e.Reason.Code != code {
		return err
	}
	magicMarker := code + ":"
	index := strings.LastIndex(e.Reason.Message, magicMarker)
	if index == -1 {
		return err
	}
	return &termsRequiredError{strings.Fields(e.Reason.Message[index+len(magicMarker):])}
}

type termsRequiredError struct {
	Terms []string
}

func (e *termsRequiredError) Error() string {
	return fmt.Sprintf("please agree to terms %q", strings.Join(e.Terms, " "))
}

func isSeriesSupported(requestedSeries string, supportedSeries []string) bool {
	for _, series := range supportedSeries {
		if series == requestedSeries {
			return true
		}
	}
	return false
}

type resolveCharmStoreEntityParams struct {
	urlStr          string
	requestedSeries string
	forceSeries     bool
	csParams        charmrepo.NewCharmStoreParams
	repoPath        string
	conf            *config.Config
}

// resolveCharmStoreEntityURL resolves the given charm or bundle URL string
// by looking it up in the appropriate charm repository.
// If it is a charm store URL, the given csParams will
// be used to access the charm store repository.
// If it is a local charm or bundle URL, the local charm repository at
// the given repoPath will be used. The given configuration
// will be used to add any necessary attributes to the repo
// and to return the charm's supported series if possible.
//
// resolveCharmStoreEntityURL also returns the charm repository holding
// the charm or bundle.
func resolveCharmStoreEntityURL(args resolveCharmStoreEntityParams) (*charm.URL, []string, charmrepo.Interface, error) {
	url, err := charm.ParseURL(args.urlStr)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	repo, err := charmrepo.InferRepository(url, args.csParams, args.repoPath)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	repo = config.SpecializeCharmRepo(repo, args.conf)

	if url.Schema == "local" && url.Series == "" {
		if defaultSeries, ok := args.conf.DefaultSeries(); ok {
			url.Series = defaultSeries
		}
		if url.Series == "" {
			possibleURL := *url
			possibleURL.Series = config.LatestLtsSeries()
			logger.Errorf("The series is not specified in the model (default-series) or with the charm. Did you mean:\n\t%s", &possibleURL)
			return nil, nil, nil, errors.Errorf("cannot resolve series for charm: %q", url)
		}
	}
	resultUrl, supportedSeries, err := repo.Resolve(url)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return resultUrl, supportedSeries, repo, nil
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
				return nil, errors.Trace(err)
			}
			m, err := csclient.authorize(curl)
			if err != nil {
				return nil, maybeTermsAgreementError(err)
			}
			if err := client.AddCharmWithAuthorization(curl, m); err != nil {
				return nil, errors.Trace(err)
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
	if curl == nil {
		return nil, errors.New("empty charm url not allowed")
	}

	client := csclient.New(csclient.Params{
		URL:          c.params.URL,
		HTTPClient:   c.params.HTTPClient,
		VisitWebPage: c.params.VisitWebPage,
	})
	endpoint := "/delegatable-macaroon"
	endpoint += "?id=" + url.QueryEscape(curl.String())

	var m *macaroon.Macaroon
	if err := client.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}

	return m, nil
}
