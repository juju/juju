// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Contains login providers required only by the Juju CLI.
package loginprovider

import (
	"context"

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
		providers: providers,
		logger:    logger,
	}
}

type tryInOrderLoginProviders struct {
	providers  []api.LoginProvider
	logger     loggo.Logger
	loginToken string
}

// Token implements the LoginProvider.Token method.
// Returns the token obtained from the last successful login provider.
// Returns an empty string if no session token was obtained.
func (p *tryInOrderLoginProviders) Token() string {
	return p.loginToken
}

// Login implements the LoginProvider.Login method.
func (p *tryInOrderLoginProviders) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	var lastError error
	for i, provider := range p.providers {
		result, err := provider.Login(ctx, caller)
		if err != nil {
			p.logger.Debugf("login error using provider %d - %s", i, err.Error())
		} else {
			p.logger.Debugf("successful login using provider %d", i)
			p.loginToken = provider.Token()
			return result, nil
		}
		lastError = err
	}
	return nil, errors.Trace(lastError)
}
