// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

var (
	loginWithClientCredentialsAPICall = func(ctx context.Context, caller base.APICaller, request interface{}, response interface{}) error {
		return caller.APICall(ctx, "Admin", 4, "", "LoginWithClientCredentials", request, response)
	}
)

// NewClientCredentialsLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the given client credentials.
func NewClientCredentialsLoginProvider(clientID, clientSecret string) *clientCredentialsLoginProvider {
	return &clientCredentialsLoginProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

type clientCredentialsLoginProvider struct {
	clientID     string
	clientSecret string
}

// Login implements the LoginProvider.Login method.
//
// It authenticates as the entity using client credentials.
// Subsequent requests on the state will act as that entity.
func (p *clientCredentialsLoginProvider) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var result params.LoginResult
	request := struct {
		ClientID     string `json:"client-id"`
		ClientSecret string `json:"client-secret"`
	}{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
	}

	err := loginWithClientCredentialsAPICall(ctx, caller, request, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewLoginResultParams(result)
}
