// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"io"
	"net/http"
	"os"

	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// TODO(ericsnow) Factor out code from cmd/juju/commands/common.go and
// cmd/envcmd/base.go into cmd/charmstore.go and cmd/apicontext.go. Then
// use those here instead of copy-and-pasting here.

// CharmstoreClient exposes the functionality of the charm store client.
type CharmstoreClient interface {
	// TODO(ericsnow) Embed github.com/juju/juju/charmstore.Client.
	io.Closer
}

///////////////////
// The charmstoreSpec code is based loosely on code in cmd/juju/commands/deploy.go.

// CharmstoreSpec provides the functionality needed to open a charm
// store client.
type CharmstoreSpec interface {
	// Connect connects to the specified charm store.
	Connect() (CharmstoreClient, error)
}

type charmstoreSpec struct {
	params charmrepo.NewCharmStoreParams
}

// newCharmstoreSpec creates a new charm store spec with default
// settings.
func newCharmstoreSpec() CharmstoreSpec {
	return &charmstoreSpec{
		params: charmrepo.NewCharmStoreParams{
			//URL:        We use the default.
			//HTTPClient: We set it later.
			VisitWebPage: httpbakery.OpenWebBrowser,
		},
	}
}

// Connect implements CharmstoreSpec.
func (cs charmstoreSpec) Connect() (CharmstoreClient, error) {
	params, apiContext, err := cs.connect()
	if err != nil {
		return nil, errors.Trace(err)
	}

	baseClient := csclient.New(csclient.Params{
		URL:          params.URL,
		HTTPClient:   params.HTTPClient,
		VisitWebPage: params.VisitWebPage,
	})

	csClient := &charmstoreClient{
		Client:     baseClient,
		apiContext: apiContext,
	}
	return csClient, nil
}

// TODO(ericsnow) Also add charmstoreSpec.Repo() -> charmrepo.Interface?

func (cs charmstoreSpec) connect() (charmrepo.NewCharmStoreParams, *apiContext, error) {
	apiContext, err := newAPIContext()
	if err != nil {
		return charmrepo.NewCharmStoreParams{}, nil, errors.Trace(err)
	}

	params := cs.params // a copy
	params.HTTPClient = apiContext.HTTPClient()
	return params, apiContext, nil
}

///////////////////
// charmstoreClient is based loosely on cmd/juju/commands/common.go.

type charmstoreClient struct {
	*csclient.Client
	*apiContext
}

// Close implements io.Closer.
func (cs *charmstoreClient) Close() error {
	return cs.apiContext.Close()
}

///////////////////
// For the most part, apiContext is copied directly from cmd/envcmd/base.go.

// newAPIContext returns a new api context, which should be closed
// when done with.
func newAPIContext() (*apiContext, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: cookieFile(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := httpbakery.NewClient()
	client.Jar = jar
	client.VisitWebPage = httpbakery.OpenWebBrowser

	return &apiContext{
		jar:    jar,
		client: client,
	}, nil
}

// apiContext is a convenience type that can be embedded wherever
// we need an API connection.
// It also stores a bakery bakery client allowing the API
// to be used using macaroons to authenticate. It stores
// obtained macaroons and discharges in a cookie jar file.
type apiContext struct {
	jar    *cookiejar.Jar
	client *httpbakery.Client
}

// Close saves the embedded cookie jar.
func (c *apiContext) Close() error {
	if err := c.jar.Save(); err != nil {
		return errors.Annotatef(err, "cannot save cookie jar")
	}
	return nil
}

// HTTPClient returns an http.Client that contains the loaded
// persistent cookie jar.
func (ctx *apiContext) HTTPClient() *http.Client {
	return ctx.client.Client
}

// cookieFile returns the path to the cookie used to store authorization
// macaroons. The returned value can be overridden by setting the
// JUJU_COOKIEFILE or GO_COOKIEFILE environment variables.
func cookieFile() string {
	if file := os.Getenv("JUJU_COOKIEFILE"); file != "" {
		return file
	}
	return cookiejar.DefaultCookieFile()
}
