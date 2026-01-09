// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loginprovider

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
)

// NewTryInOrderLoginProvider returns a login provider that will attempt to
// log in using all the specified login providers in sequence - results
// of the first on that succeeds will be returned.
// This login provider should only be used when connecting to a controller
// for the first time when we still don't know which login method.
func NewTryInOrderLoginProvider(logger loggo.Logger, providers ...api.LoginProvider) api.LoginProvider {
	return &tryInOrderLoginProviders{
		providers:  providers,
		logger:     logger,
		authHeader: missingHeader,
	}
}

func missingHeader() (http.Header, error) {
	return nil, api.ErrorLoginFirst
}

type tryInOrderLoginProviders struct {
	providers  []api.LoginProvider
	logger     loggo.Logger
	authHeader func() (http.Header, error)
}

func (p *tryInOrderLoginProviders) String() string {
	return "TryInOrderLoginProvider"
}

// AuthHeader implements the [LoginProvider.AuthHeader] method.
// It attempts to retrieve the auth header from the last successful login provider.
// If login was never attempted/successful, an ErrorLoginFirst error is returned.
func (p *tryInOrderLoginProviders) AuthHeader() (http.Header, error) {
	return p.authHeader()
}

// Login implements the LoginProvider.Login method.
func (p *tryInOrderLoginProviders) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	for _, provider := range p.providers {
		result, err := provider.Login(ctx, caller)
		if err != nil {
			p.logger.Debugf("login error using provider %s - %s", provider, err.Error())
		} else {
			p.logger.Debugf("successful login using provider %s", provider)
			p.authHeader = func() (http.Header, error) { return provider.AuthHeader() }
			return result, nil
		}
	}
	return nil, errors.New("login failed (use --debug for more information)")
}
