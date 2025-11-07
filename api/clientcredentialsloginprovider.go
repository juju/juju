// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net/http"
	"os"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

var (
	loginWithClientCredentialsAPICall = func(caller base.APICaller, request interface{}, response interface{}) error {
		return caller.APICall("Admin", 4, "", "LoginWithClientCredentials", request, response)
	}
)

const (
	// clientIDEnvVar is the environment variable used to specify the client ID
	// for client credentials authentication.
	clientIDEnvVar = "JUJU_CLIENT_ID"
	// clientSecretEnvVar is the environment variable used to specify the client
	// secret for client credentials authentication.
	clientSecretEnvVar = "JUJU_CLIENT_SECRET"
)

// NewClientCredentialsLoginProviderFromEnvironment returns a LoginProvider implementation that
// authenticates the entity with the client credentials retrieved from the environment.
func NewClientCredentialsLoginProviderFromEnvironment(f func()) *clientCredentialsLoginProvider {
	clientID := os.Getenv(clientIDEnvVar)
	clientSecret := os.Getenv(clientSecretEnvVar)

	return &clientCredentialsLoginProvider{
		clientID:     clientID,
		clientSecret: clientSecret,

		afterLoginCallback: f,
	}
}

// NewClientCredentialsLoginProvider returns a LoginProvider implementation that
// authenticates the entity with the given client credentials.
func NewClientCredentialsLoginProvider(clientID, clientSecret string) *clientCredentialsLoginProvider {
	return &clientCredentialsLoginProvider{
		clientID:           clientID,
		clientSecret:       clientSecret,
		afterLoginCallback: nil,
	}
}

type clientCredentialsLoginProvider struct {
	clientID     string
	clientSecret string

	afterLoginCallback func()
}

// AuthHeader implements the [LoginProvider.AuthHeader] method.
// It returns an HTTP header with basic auth set.
func (p *clientCredentialsLoginProvider) AuthHeader() (http.Header, error) {
	return jujuhttp.BasicAuthHeader(p.clientID, p.clientSecret), nil
}

// Login implements the LoginProvider.Login method.
//
// It authenticates as the entity using client credentials.
// Subsequent requests on the state will act as that entity.
func (p *clientCredentialsLoginProvider) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	if !p.clientIdAndSecretSet() {
		return nil, errors.New("both client id and client secret must be set")
	}

	var result params.LoginResult
	request := struct {
		ClientID     string `json:"client-id"`
		ClientSecret string `json:"client-secret"`
	}{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
	}

	err := loginWithClientCredentialsAPICall(caller, request, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if p.afterLoginCallback != nil {
		p.afterLoginCallback()
	}

	return NewLoginResultParams(result)
}

func (p *clientCredentialsLoginProvider) clientIdAndSecretSet() bool {
	return p.clientID != "" && p.clientSecret != ""
}
