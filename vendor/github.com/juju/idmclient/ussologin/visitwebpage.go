// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ussologin

import (
	"net/http"
	"net/url"

	"github.com/juju/usso"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// VisitWebPage returns a function which will allow authentication via USSO
// OAuth.  If UbuntuSSO OAuth login is not available then this function falls
// back to httpbakery.OpenWebBrowser.  The user will be prompted for username,
// password and any two factor authentication code via the command line.
// Existing oauth tokens can be obtained, or new ones stored If non-nil, the
// given TokenStore is used to store the oauth token obtained during the login
// process so that less interaction may be required in future.
func VisitWebPage(tokenName string, client *http.Client, filler form.Filler, store TokenStore) func(*url.URL) error {
	visitor := NewVisitor(tokenName, filler, store)
	return func(u *url.URL) error {
		methodURLs, err := httpbakery.GetInteractionMethods(client, u)
		if err != nil {
			return errgo.Mask(err)
		}
		return visitor.VisitWebPage(&httpbakery.Client{Client: client}, methodURLs)
	}
}

// Visitor is an httpbakery.Visitor that will login using Ubuntu SSO
// OAuth if it is supported by the discharger.
type Visitor struct {
	tokenName string
	filler    form.Filler
	store     TokenStore
}

// NewVisitor creates a new Visitor that will attempt to interact using
// an Ubuntu SSO OAuth token. If there is a token stored in store then
// that will be used. Otherwise filler will be used to ineract with the
// user and the credentials will be sent to Ubuntu SSO to create a token
// named tokenName. That token will be stored in store if possible and
// used to interact with the discharger.
func NewVisitor(tokenName string, filler form.Filler, store TokenStore) *Visitor {
	return &Visitor{
		tokenName: tokenName,
		filler:    filler,
		store:     store,
	}
}

// VisitWebPage implements httpbakery.Visitor.VisitWebPage by attempting
// to obtain an Ubuntu SSO OAuth token and use that to sign a request to
// the identity manager. If Ubuntu SSO returns an error when attempting
// to obtain the token the error returned will have a cause of type
// *usso.Error.
func (v *Visitor) VisitWebPage(client *httpbakery.Client, methodURLs map[string]*url.URL) error {
	if methodURLs["usso_oauth"] == nil {
		return httpbakery.ErrMethodNotSupported
	}
	var tok *usso.SSOData
	var err error
	if v.store != nil {
		// Ignore any error from the store, we'll attempt to get
		// the token from Ubuntu SSO.
		tok, _ = v.store.Get()
	}
	if tok == nil {
		tok, err = GetToken(v.filler, v.tokenName)
		if err != nil {
			return errgo.Mask(err, isUSSOError)
		}
		if v.store != nil {
			// Ignore any error from the store, we can still
			// log in, the user will just be prompted for
			// credentials again next time.
			v.store.Put(tok)
		}
	}
	return LoginWithToken(client.Client, methodURLs["usso_oauth"].String(), tok)
}
