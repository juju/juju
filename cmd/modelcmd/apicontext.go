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
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	bakeryV2 "gopkg.in/macaroon-bakery.v2-unstable/bakery"
	httpbakeryV2 "gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	macaroon "gopkg.in/macaroon.v1"
	macaroonV2 "gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/jujuclient"
)

// apiContext holds the context required for making connections to
// APIs used by juju.
type apiContext struct {
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

// newAPIContext returns an API context that will use the given
// context for user interactions when authorizing.
// The returned API context must be closed after use.
//
// If ctxt is nil, no command-line authorization
// will be supported.
//
// This function is provided for use by commands that cannot use
// CommandBase. Most clients should use that instead.
func newAPIContext(ctxt *cmd.Context, opts *AuthOpts, store jujuclient.CookieStore, controllerName string) (*apiContext, error) {
	jar0, err := store.CookieJar(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The JUJU_USER_DOMAIN environment variable specifies
	// the preferred user domain when discharging third party caveats.
	// We set up a cookie jar that will send it to all sites because
	// we don't know where the third party might be.
	jar := &domainCookieJar{
		CookieJar: jar0,
		domain:    os.Getenv("JUJU_USER_DOMAIN"),
	}
	var visitors []httpbakery.Visitor
	if ctxt != nil && opts != nil && opts.NoBrowser {
		filler := &form.IOFiller{
			In:  ctxt.Stdin,
			Out: ctxt.Stdout,
		}
		newVisitor := ussologin.NewVisitor("juju", filler, jujuclient.NewTokenStore())
		visitors = append(visitors, v2ToV1Visitor{v2Visitor: newVisitor})
	} else {
		visitors = append(visitors, httpbakery.WebBrowserVisitor)
	}
	return &apiContext{
		jar:            jar,
		webPageVisitor: httpbakery.NewMultiVisitor(visitors...),
	}, nil
}

// CookieJar returns the cookie jar used to make
// HTTP requests.
func (ctx *apiContext) CookieJar() http.CookieJar {
	return ctx.jar
}

// NewBakeryClient returns a new httpbakery.Client, using the API context's
// persistent cookie jar and web page visitor.
func (ctx *apiContext) NewBakeryClient() *httpbakery.Client {
	client := httpbakery.NewClient()
	client.Jar = ctx.jar
	client.WebPageVisitor = ctx.webPageVisitor
	return client
}

// Close closes the API context, saving any cookies to the
// persistent cookie jar.
func (ctxt *apiContext) Close() error {
	if err := ctxt.jar.Save(); err != nil {
		return errors.Annotatef(err, "cannot save cookie jar")
	}
	return nil
}

const domainCookieName = "domain"

// domainCookieJar implements a variant of CookieJar that
// always includes a domain cookie regardless of the site.
type domainCookieJar struct {
	jujuclient.CookieJar
	// domain holds the value of the domain cookie.
	domain string
}

// Cookies implements http.CookieJar.Cookies by
// adding the domain cookie when the domain is non-empty.
func (j *domainCookieJar) Cookies(u *url.URL) []*http.Cookie {
	cookies := j.CookieJar.Cookies(u)
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

type v2ToV1Visitor struct {
	v2Visitor httpbakeryV2.Visitor
}

// VisitWebPage implements httpbakery.WebPageVisitor by making a call to a httpbakeryV2.WebPageVisitor.
func (v v2ToV1Visitor) VisitWebPage(client *httpbakery.Client, methodURLs map[string]*url.URL) error {
	key := v1ToV2Key(client.Key)
	var acquirer httpbakeryV2.DischargeAcquirer
	if client.DischargeAcquirer != nil {
		acquirer = v1ToV2DischargeAcquirer{client.DischargeAcquirer}
	}
	var visitor httpbakeryV2.Visitor
	if client.WebPageVisitor != nil {
		visitor = v1ToV2Visitor{
			visitor: client.WebPageVisitor,
			client:  client,
		}
	}
	clientV2 := &httpbakeryV2.Client{
		Client:            client.Client,
		WebPageVisitor:    visitor,
		VisitWebPage:      client.VisitWebPage,
		Key:               key,
		DischargeAcquirer: acquirer,
	}
	return v.v2Visitor.VisitWebPage(clientV2, methodURLs)
}

type v1ToV2Visitor struct {
	visitor httpbakery.Visitor
	client  *httpbakery.Client
}

func (v v1ToV2Visitor) VisitWebPage(client *httpbakeryV2.Client, methodURLs map[string]*url.URL) error {
	return v.visitor.VisitWebPage(v.client, methodURLs)
}

func v1ToV2Key(key *bakery.KeyPair) *bakeryV2.KeyPair {
	if key == nil {
		return nil
	}
	var keyV2 bakeryV2.KeyPair
	copy(keyV2.Public.Key[:], key.Public.Key[:])
	copy(keyV2.Private.Key[:], key.Private.Key[:])
	return &keyV2
}

type v1ToV2DischargeAcquirer struct {
	acquirer httpbakery.DischargeAcquirer
}

func (a v1ToV2DischargeAcquirer) AcquireDischarge(cavV2 macaroonV2.Caveat) (*macaroonV2.Macaroon, error) {
	m, err := a.acquirer.AcquireDischarge("", macaroon.Caveat{
		Id:       string(cavV2.Id),
		Location: cavV2.Location,
	})
	if err != nil {
		return nil, err
	}
	data, err := m.MarshalBinary()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot marshal v1 macaroon")
	}
	var mV2 macaroonV2.Macaroon
	if err := mV2.UnmarshalBinary(data); err != nil {
		return nil, errors.Annotatef(err, "cannot unmarshal v1 macaroon into v2")
	}
	return &mV2, nil
}
