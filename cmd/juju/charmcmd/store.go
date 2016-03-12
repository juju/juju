// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"net/http"
	"net/url"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/idmclient/ussologin"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/jujuclient"
)

// TODO(ericsnow) Factor out code from cmd/juju/commands/common.go and
// cmd/envcmd/base.go into cmd/charmstore.go and cmd/apicontext.go. Then
// use those here instead of copy-and-pasting here.

///////////////////
// The charmstoreSpec code is based loosely on code in cmd/juju/commands/deploy.go.

// CharmstoreSpec provides the functionality needed to open a charm
// store client.
type CharmstoreSpec interface {
	// Connect connects to the specified charm store.
	Connect(ctx *cmd.Context) (*charmstore.Client, error)
}

type charmstoreSpec struct{}

// newCharmstoreSpec creates a new charm store spec with default
// settings.
func newCharmstoreSpec() CharmstoreSpec {
	return &charmstoreSpec{}
}

// Connect implements CharmstoreSpec.
func (cs charmstoreSpec) Connect(ctx *cmd.Context) (*charmstore.Client, error) {
	visitWebPage := httpbakery.OpenWebBrowser
	if ctx != nil {
		filler := &form.IOFiller{
			In:  ctx.Stdin,
			Out: ctx.Stderr,
		}
		visitWebPage = ussologin.VisitWebPage(
			filler,
			&http.Client{},
			jujuclient.NewTokenStore(),
		)
	}
	apiContext, err := newAPIContext(visitWebPage)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := charmstore.NewClient(charmstore.ClientConfig{
		charmrepo.NewCharmStoreParams{
			HTTPClient:   apiContext.HTTPClient(),
			VisitWebPage: visitWebPage,
		},
	})
	return client, nil
}

// TODO(ericsnow) Also add charmstoreSpec.Repo() -> charmrepo.Interface?

///////////////////
// For the most part, apiContext is copied directly from cmd/envcmd/base.go.

// newAPIContext returns a new api context, which should be closed
// when done with.
func newAPIContext(visitWebPage func(*url.URL) error) (*apiContext, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: cookieFile(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := httpbakery.NewClient()
	client.Jar = jar
	client.VisitWebPage = visitWebPage

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
