// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

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

	var controllerAccess string
	var modelAccess string
	var tag names.Tag
	if result.UserInfo != nil {
		tag, err = names.ParseTag(result.UserInfo.Identity)
		if err != nil {
			return nil, errors.Trace(err)
		}
		controllerAccess = result.UserInfo.ControllerAccess
		modelAccess = result.UserInfo.ModelAccess
	}
	servers := params.ToMachineHostsPorts(result.Servers)
	serverVersion, err := version.Parse(result.ServerVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &LoginResultParams{
		tag:              tag,
		modelTag:         result.ModelTag,
		controllerTag:    result.ControllerTag,
		servers:          servers,
		publicDNSName:    result.PublicDNSName,
		facades:          result.Facades,
		modelAccess:      modelAccess,
		controllerAccess: controllerAccess,
		serverVersion:    serverVersion,
	}, nil
}
