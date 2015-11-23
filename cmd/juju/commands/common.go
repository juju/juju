// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
)

// destroyPreparedEnviron destroys the environment and logs an error
// if it fails.
var destroyPreparedEnviron = destroyPreparedEnvironProductionFunc

var logger = loggo.GetLogger("juju.cmd.juju")

func destroyPreparedEnvironProductionFunc(
	ctx *cmd.Context,
	env environs.Environ,
	store configstore.Storage,
	action string,
) {
	ctx.Infof("%s failed, destroying environment", action)
	if err := environs.Destroy(env, store); err != nil {
		logger.Errorf("the environment could not be destroyed: %v", err)
	}
}

var destroyEnvInfo = destroyEnvInfoProductionFunc

func destroyEnvInfoProductionFunc(
	ctx *cmd.Context,
	cfgName string,
	store configstore.Storage,
	action string,
) {
	ctx.Infof("%s failed, cleaning up the environment.", action)
	if err := environs.DestroyInfo(cfgName, store); err != nil {
		logger.Errorf("the environment jenv file could not be cleaned up: %v", err)
	}
}

// environFromName loads an existing environment or prepares a new
// one. If there are no errors, it returns the environ and a closure to
// clean up in case we need to further up the stack. If an error has
// occurred, the environment and cleanup function will be nil, and the
// error will be filled in.
var environFromName = environFromNameProductionFunc

func environFromNameProductionFunc(
	ctx *cmd.Context,
	envName string,
	action string,
	ensureNotBootstrapped func(environs.Environ) error,
) (env environs.Environ, cleanup func(), err error) {

	store, err := configstore.Default()
	if err != nil {
		return nil, nil, err
	}

	envExisted := false
	if environInfo, err := store.ReadInfo(envName); err == nil {
		envExisted = true
		logger.Warningf(
			"ignoring environments.yaml: using bootstrap config in %s",
			environInfo.Location(),
		)
	} else if !errors.IsNotFound(err) {
		return nil, nil, err
	}

	cleanup = func() {
		// Distinguish b/t removing the jenv file or tearing down the
		// environment. We want to remove the jenv file if preparation
		// was not successful. We want to tear down the environment
		// only in the case where the environment didn't already
		// exist.
		if env == nil {
			logger.Debugf("Destroying environment info.")
			destroyEnvInfo(ctx, envName, store, action)
		} else if !envExisted && ensureNotBootstrapped(env) != environs.ErrAlreadyBootstrapped {
			logger.Debugf("Destroying environment.")
			destroyPreparedEnviron(ctx, env, store, action)
		}
	}

	if env, err = environs.PrepareFromName(envName, envcmd.BootstrapContext(ctx), store); err != nil {
		return nil, cleanup, err
	}

	return env, cleanup, err
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
			logger.Errorf("The series is not specified in the environment (default-series) or with the charm. Did you mean:\n\t%s", &possibleURL)
			return nil, nil, nil, errors.Errorf("cannot resolve series for charm: %q", url)
		}
	}
	resultUrl, supportedSeries, err := repo.Resolve(url)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return resultUrl, supportedSeries, repo, nil
}

func isSeriesSupported(requestedSeries string, supportedSeries []string) bool {
	for _, series := range supportedSeries {
		if series == requestedSeries {
			return true
		}
	}
	return false
}

// termsAgreementError returns err as a *termsAgreementError
// if it has a "terms agreement request" error code, otherwise
// it returns err unchanged.
func termsAgreementError(err error) error {
	e, ok := err.(*httpbakery.DischargeError)
	if !ok {
		return nil
	}
	if e.Reason == nil {
		return err
	}
	code := "term agreement required"
	if e.Reason.Code != httpbakery.ErrorCode(code) {
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
				return nil, termsAgreementError(errors.Cause(err))
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
	client := csclient.New(csclient.Params{
		URL:          c.params.URL,
		HTTPClient:   c.params.HTTPClient,
		VisitWebPage: c.params.VisitWebPage,
	})
	endpoint := "/delegatable-macaroon"
	if curl != nil {
		query := url.Values{}
		query.Add("id", curl.String())
		endpoint = endpoint + "?" + query.Encode()
	}
	var m *macaroon.Macaroon
	if err := client.Get(endpoint, &m); err != nil {
		return nil, errors.Trace(err)
	}
	if err := m.AddFirstPartyCaveat("is-entity " + curl.String()); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
