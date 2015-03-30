// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"net/http"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
)

const (
	driverScopes = "https://www.googleapis.com/auth/compute " +
		"https://www.googleapis.com/auth/devstorage.full_control"

	tokenURL = "https://accounts.google.com/o/oauth2/token"

	authURL = "https://accounts.google.com/o/oauth2/auth"
)

// newTransport builds a new network transport that wraps requests
// with the GCE-required OAuth authentication/authorization. The
// transport is built using the Auth values. The following GCE access
// scopes are used:
//   https://www.googleapis.com/auth/compute
//   https://www.googleapis.com/auth/devstorage.full_control
func newTransport(creds Credentials) (*oauth.Transport, error) {
	token, err := newToken(creds, driverScopes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	transport := oauth.Transport{
		Config: &oauth.Config{
			ClientId: creds.ClientID,
			Scope:    driverScopes,
			TokenURL: tokenURL,
			AuthURL:  authURL,
		},
		Token: token,
	}
	return &transport, nil
}

// newToken generates a new OAuth token for use in the OAuth-wrapping
// network transport and returns it. This involves network calls to the
// GCE OAuth API.
var newToken = func(creds Credentials, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(creds.ClientEmail, scopes, creds.PrivateKey)
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	if err != nil {
		msg := "retrieving auth token for %s"
		return nil, errors.Annotatef(err, msg, creds.ClientEmail)
	}
	return token, nil
}

// newConnection opens a new low-level connection to the GCE API using
// the Auth's data and returns it. This includes building the
// OAuth-wrapping network transport.
func newConnection(creds Credentials) (*compute.Service, error) {
	transport, err := newTransport(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}
	service, err := newService(transport)
	return service, errors.Trace(err)
}

// newService is a simple wrapper around compute.New for use in testing.
var newService = func(transport *oauth.Transport) (*compute.Service, error) {
	return compute.New(transport.Client())
}
