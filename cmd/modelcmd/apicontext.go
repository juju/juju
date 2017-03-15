// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"net/http"
	"net/url"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/idmclient/ussologin"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/jujuclient"
)

// APIContext holds the context required for making connections to
// APIs used by juju.
type APIContext struct {
	// jar holds the internal version of the cookie jar - it has
	// methods that clients should not use, such as Save.
	jar            *domainCookieJar
	webPageVisitor httpbakery.Visitor
}

// AuthOpts holds flags relating to authentication.
type AuthOpts struct {
	// NoBrowser specifies that web-browser-based auth should
	// not be used when authenticating.
	NoBrowser bool
}

func (o *AuthOpts) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&o.NoBrowser, "B", false, "Do not use web browser for authentication")
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
	jar0, err := cookiejar.New(&cookiejar.Options{
		Filename: cookieFile(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The JUJU_USER_DOMAIN environment variable specifies
	// the preferred user domain when discharging third party caveats.
	// We set up a cookie jar that will send it to all sites because
	// we don't know where the third party might be.
	jar := &domainCookieJar{
		Jar:    jar0,
		domain: os.Getenv("JUJU_USER_DOMAIN"),
	}
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
	return &APIContext{
		jar:            jar,
		webPageVisitor: httpbakery.NewMultiVisitor(visitors...),
	}, nil
}

// CookieJar returns the cookie jar used to make
// HTTP requests.
func (ctx *APIContext) CookieJar() http.CookieJar {
	return ctx.jar
}

// NewBakeryClient returns a new httpbakery.Client, using the API context's
// persistent cookie jar and web page visitor.
func (ctx *APIContext) NewBakeryClient() *httpbakery.Client {
	client := httpbakery.NewClient()
	client.Jar = ctx.jar
	client.WebPageVisitor = ctx.webPageVisitor
	return client
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
	if err := ctxt.jar.Save(); err != nil {
		return errors.Annotatef(err, "cannot save cookie jar")
	}
	return nil
}

const domainCookieName = "domain"

// domainCookieJar implements a variant of CookieJar that
// always includes a domain cookie regardless of the site.
type domainCookieJar struct {
	*cookiejar.Jar
	// domain holds the value of the domain cookie.
	domain string
}

// Cookies implements http.CookieJar.Cookies by
// adding the domain cookie when the domain is non-empty.
func (j *domainCookieJar) Cookies(u *url.URL) []*http.Cookie {
	cookies := j.Jar.Cookies(u)
	if j.domain == "" {
		return cookies
	}
	// Allow the site to override if it wants to.
	for _, c := range cookies {
		if c.Name == domainCookieName {
			return cookies
		}
	}
	return append(cookies, &http.Cookie{
		Name:  domainCookieName,
		Value: j.domain,
	})
}
