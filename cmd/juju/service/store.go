// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
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

// charmURLResolver holds the information necessary to
// resolve charm and bundle URLs.
type charmURLResolver struct {
	// csRepo holds the repository to use for charmstore charms.
	csRepo charmrepo.Interface

	// repoPath holds the path to the local charm repository directory.
	repoPath string

	// conf holds the current model configuration.
	conf *config.Config
}

func newCharmURLResolver(conf *config.Config, csClient *csclient.Client, repoPath string) *charmURLResolver {
	r := &charmURLResolver{
		csRepo:   charmrepo.NewCharmStoreFromClient(csClient),
		repoPath: repoPath,
		conf:     conf,
	}
	return r
}

// TODO(ericsnow) Return charmstore.CharmID from resolve()?

// resolve resolves the given given charm or bundle URL
// string by looking it up in the appropriate charm repository. If it is
// a charm store URL, the given csParams will be used to access the
// charm store repository. If it is a local charm or bundle URL, the
// local charm repository at the given repoPath will be used. The given
// configuration will be used to add any necessary attributes to the
// repo and to return the charm's supported series if possible.
//
// It returns the fully resolved URL, any series supported by the entity,
// and the repository that holds it.
func (r *charmURLResolver) resolve(urlStr string) (*charm.URL, csparams.Channel, []string, charmrepo.Interface, error) {
	var noChannel csparams.Channel
	url, err := charm.ParseURL(urlStr)
	if err != nil {
		return nil, noChannel, nil, nil, errors.Trace(err)
	}
	switch url.Schema {
	case "cs":
		repo := config.SpecializeCharmRepo(r.csRepo, r.conf).(*charmrepo.CharmStore)

		resultUrl, channel, supportedSeries, err := repo.ResolveWithChannel(url)
		if err != nil {
			return nil, noChannel, nil, nil, errors.Trace(err)
		}
		return resultUrl, channel, supportedSeries, repo, nil
	case "local":
		if url.Series == "" {
			if defaultSeries, ok := r.conf.DefaultSeries(); ok {
				url.Series = defaultSeries
			}
		}
		if url.Series == "" {
			possibleURL := *url
			possibleURL.Series = config.LatestLtsSeries()
			logger.Errorf("The series is not specified in the model (default-series) or with the charm. Did you mean:\n\t%s", &possibleURL)
			return nil, noChannel, nil, nil, errors.Errorf("cannot resolve series for charm: %q", url)
		}
		repo, err := charmrepo.NewLocalRepository(r.repoPath)
		if err != nil {
			return nil, noChannel, nil, nil, errors.Mask(err)
		}
		repo = config.SpecializeCharmRepo(repo, r.conf)

		resultUrl, supportedSeries, err := repo.Resolve(url)
		if err != nil {
			return nil, noChannel, nil, nil, errors.Trace(err)
		}
		return resultUrl, noChannel, supportedSeries, repo, nil
	default:
		return nil, noChannel, nil, nil, errors.Errorf("unknown schema for charm reference %q", urlStr)
	}
}

// TODO(ericsnow) Return charmstore.CharmID from addCharmFromURL()?

// addCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state. For non-public charm URLs, this function also
// handles the macaroon authorization process using the given csClient.
// The resulting charm URL of the added charm is displayed on stdout.
//
// The repo holds the charm repository associated with with the URL
// by resolveCharmStoreEntityURL.
func addCharmFromURL(client *api.Client, curl *charm.URL, channel csparams.Channel, repo charmrepo.Interface) (*charm.URL, *macaroon.Macaroon, error) {
	var csMac *macaroon.Macaroon
	switch curl.Schema {
	case "local":
		ch, err := repo.Get(curl)
		if err != nil {
			return nil, nil, err
		}
		stateCurl, err := client.AddLocalCharm(curl, ch)
		if err != nil {
			return nil, nil, err
		}
		curl = stateCurl
	case "cs":
		repo, ok := repo.(*charmrepo.CharmStore)
		if !ok {
			return nil, nil, errors.Errorf("(cannot happen) cs-schema URL with unexpected repo type %T", repo)
		}
		csClient := repo.Client()
		if err := client.AddCharm(curl, channel); err != nil {
			if !params.IsCodeUnauthorized(err) {
				return nil, nil, errors.Trace(err)
			}
			m, err := authorizeCharmStoreEntity(csClient, curl)
			if err != nil {
				return nil, nil, maybeTermsAgreementError(err)
			}
			if err := client.AddCharmWithAuthorization(curl, channel, m); err != nil {
				return nil, nil, errors.Trace(err)
			}
			csMac = m
		}
	default:
		return nil, nil, fmt.Errorf("unsupported charm URL schema: %q", curl.Schema)
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
