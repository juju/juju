// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"net/http"
	"os"

	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/charmstore"
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
	Connect() (*charmstore.Client, error)
}

type charmstoreSpec struct {
	config charmstore.ClientConfig
}

// newCharmstoreSpec creates a new charm store spec with default
// settings.
func newCharmstoreSpec() CharmstoreSpec {
	var config charmstore.ClientConfig
	// We use the default for URL and set HTTPClient later.
	config.VisitWebPage = httpbakery.OpenWebBrowser
	return &charmstoreSpec{
		config: config,
	}
}

// Connect implements CharmstoreSpec.
func (cs charmstoreSpec) Connect() (*charmstore.Client, error) {
	config, apiContext, err := cs.connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	csClient := charmstore.NewClient(config)
	csClient.Closer = apiContext
	return csClient, nil
}

// TODO(ericsnow) Also add charmstoreSpec.Repo() -> charmrepo.Interface?

func (cs charmstoreSpec) connect() (charmstore.ClientConfig, *apiContext, error) {
	apiContext, err := newAPIContext()
	if err != nil {
		return charmstore.ClientConfig{}, nil, errors.Trace(err)
	}

	config := cs.config // a copy
	config.HTTPClient = apiContext.HTTPClient()
	return config, apiContext, nil
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
