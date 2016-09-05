// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/idmclient/ussologin"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/cmd"
	"github.com/juju/juju/jujuclient"
)

// APIContext holds the context required for making connections to
// APIs used by juju.
type APIContext struct {
	Jar          *cookiejar.Jar
	BakeryClient *httpbakery.Client
}

// AuthOpts holds flags relating to authentication.
type AuthOpts struct {
	// NoBrowser specifies that web-browser-based auth should
	// not be used when authenticating.
	NoBrowser bool
}

func (o *AuthOpts) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&o.NoBrowser, "B", false, "do not use web browser for authentication")
	f.BoolVar(&o.NoBrowser, "no-browser-login", false, "")
}

// NewAPIContext returns an API context that will use the given
// context for user interactions when authorizing.
// The returned API context must be closed after use.
//
// If ctxt is nil, no command-line authorization
// will be supported.
//
// This function is provided for use by commands that cannot use
// JujuCommandBase. Most clients should use that instead.
func NewAPIContext(ctxt *cmd.Context, opts *AuthOpts) (*APIContext, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: cookieFile(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := httpbakery.NewClient()
	client.Jar = jar
	var visitors []httpbakery.Visitor

	if ctxt != nil && opts != nil && opts.NoBrowser {
		filler := &form.IOFiller{
			In:  ctxt.Stdin,
			Out: ctxt.Stdout,
		}
		visitors = append(visitors, ussologin.NewVisitor("juju", filler, jujuclient.NewTokenStore()))
	} else {
		visitors = append(visitors, httpbakery.WebBrowserVisitor)
	}
	client.WebPageVisitor = httpbakery.NewMultiVisitor(visitors...)
	return &APIContext{
		Jar:          jar,
		BakeryClient: client,
	}, nil
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

// Close closes the API context, saving any cookies to the
// persistent cookie jar.
func (ctxt *APIContext) Close() error {
	if err := ctxt.Jar.Save(); err != nil {
		return errors.Annotatef(err, "cannot save cookie jar")
	}
	return nil
}
