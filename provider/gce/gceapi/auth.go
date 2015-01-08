// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"net/http"
	"net/mail"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

const (
	driverScopes = "https://www.googleapis.com/auth/compute " +
		"https://www.googleapis.com/auth/devstorage.full_control"

	tokenURL = "https://accounts.google.com/o/oauth2/token"

	authURL = "https://accounts.google.com/o/oauth2/auth"
)

type Auth struct {
	ClientID    string
	ClientEmail string
	PrivateKey  []byte
}

func (ga Auth) Validate() error {
	if ga.ClientID == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientID}
	}
	if ga.ClientEmail == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientEmail}
	} else if _, err := mail.ParseAddress(ga.ClientEmail); err != nil {
		err = errors.Trace(err)
		return &config.InvalidConfigValue{OSEnvClientEmail, ga.ClientEmail, err}
	}
	if len(ga.PrivateKey) == 0 {
		return &config.InvalidConfigValue{Key: OSEnvPrivateKey}
	}
	return nil
}

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

func (ga Auth) newConnection() (*compute.Service, error) {
	transport, err := ga.newTransport()
	if err != nil {
		return nil, errors.Trace(err)
	}
	service, err := newService(transport)
	return service, errors.Trace(err)
}

var newService = func(transport *oauth.Transport) (*compute.Service, error) {
	return compute.New(transport.Client())
}
