// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
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
}

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	w.CommandBase.setAPIContext(&apiContext{})
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

// newAPIContext returns a new api context, which should be closed
// when done with.
func newAPIContext() (*apiContext, error) {
	jar, err := cookiejar.New(nil)
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
type apiContext struct {
	jar    *cookiejar.Jar
	client *httpbakery.Client
}

// Close saves the embedded cookie jar.
func (c *apiContext) Close() error {
	return c.jar.Save()
}

func (c *apiContext) DialOpts() api.DialOpts {
	opts := api.DefaultDialOpts()
	opts.BakeryClient = c.client
	return opts
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
	return juju.NewAPIClientFromName(name, ctx.client)
}
