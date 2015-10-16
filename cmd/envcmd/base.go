// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"net/http"
	"os"
	"path"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	"github.com/juju/utils"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/juju"
)

var errNoNameSpecified = errors.New("no name specified")

// CommandBase extends cmd.Command with a setAPIContext method.
type CommandBase interface {
	cmd.Command

	setAPIContext(ctx *apiContext)
}

// JujuCommandBase is a convenience type for embedding that need
// an API connection.
type JujuCommandBase struct {
	cmd.CommandBase
	*apiContext
}

func (c *JujuCommandBase) setAPIContext(ctx *apiContext) {
	c.apiContext = ctx
}

// WrapBase wraps the specified CommandBase, returning a Command
// that proxies to each of the CommandBase methods.
func WrapBase(c CommandBase) cmd.Command {
	return &baseCommandWrapper{
		CommandBase: c,
	}
}

type baseCommandWrapper struct {
	CommandBase
	*apiContext
}

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	apiCtx, err := newAPIContext()
	if err != nil {
		return errors.Trace(err)
	}
	w.apiContext = apiCtx
	w.CommandBase.setAPIContext(w.apiContext)
	defer w.apiContext.Close()

	return w.CommandBase.Run(ctx)
}

// SetFlags implements Command.SetFlags.
func (w *baseCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	w.CommandBase.SetFlags(f)
}

// Init implements Command.Init.
func (w *baseCommandWrapper) Init(args []string) error {
	return w.CommandBase.Init(args)
}

// cookieFile returns the path to the cookie used to store authorization
// macaroons. The returned value can be overridden by setting the
// JUJU_COOKIEFILE environment variable.
func cookieFile() string {
	if file := os.Getenv("JUJU_COOKIEFILE"); file != "" {
		return file
	}
	return path.Join(utils.Home(), ".go-cookies")
}

// newAPIContext returns a new api context, which should be closed
// when done with.
func newAPIContext() (*apiContext, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = jar.Load(cookieFile())
	if err != nil {
		return nil, errors.Annotate(err, "failed to load cookies")
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
	return c.jar.Save()
}

// APIOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (ctx *apiContext) APIOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}

// NewAPIRoot establishes a connection to the API server for
// the named environment.
func (ctx *apiContext) NewAPIRoot(name string) (api.Connection, error) {
	if name == "" {
		return nil, errors.Trace(errNoNameSpecified)
	}
	return juju.NewAPIFromName(name, ctx.client)
}

// NewAPIClient returns an api.Client connecte to the API server
// for the named environment.
func (ctx *apiContext) NewAPIClient(name string) (*api.Client, error) {
	root, err := ctx.NewAPIRoot(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root.Client(), nil
}

// HTTPClient returns an http.Client that contains the loaded
// persistent cookie jar.
func (ctx *apiContext) HTTPClient() (*http.Client, error) {
	if ctx.client == nil || ctx.client.Client == nil {
		return nil, errors.New("http client not initialized")
	}
	return ctx.client.Client, nil
}
