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

// Auth holds the information needed to authenticate on GCE.
type Auth struct {
	// ClientID is the GCE account's OAuth ID. It is part of the OAuth
	// config used in the OAuth-wrapping network transport.
	ClientID string
	// ClientEmail is the email address associatd with the GCE account.
	// It is used to generate a new OAuth token to use in the
	// OAuth-wrapping network transport.
	ClientEmail string
	// PrivateKey is the private key that matches the public key
	// associatd with the GCE account. It is used to generate a new
	// OAuth token to use in the OAuth-wrapping network transport.
	PrivateKey []byte
}

// newTransport builds a new network transport that wraps requests
// with the GCE-required OAuth authentication/authorization. The
// transport is built using the Auth values. The following GCE access
// scopes are used:
//   https://www.googleapis.com/auth/compute
//   https://www.googleapis.com/auth/devstorage.full_control
func (ga Auth) newTransport() (*oauth.Transport, error) {
	token, err := newToken(ga, driverScopes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	transport := oauth.Transport{
		Config: &oauth.Config{
			ClientId: ga.ClientID,
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
var newToken = func(auth Auth, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(auth.ClientEmail, scopes, auth.PrivateKey)
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	if err != nil {
		msg := "retrieving auth token for %s"
		return nil, errors.Annotatef(err, msg, auth.ClientEmail)
	}
	return token, nil
}

// newConnection opens a new low-level connection to the GCE API using
// the Auth's data and returns it. This includes building the
// OAuth-wrapping network transport.
func (ga Auth) newConnection() (*compute.Service, error) {
	transport, err := ga.newTransport()
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
