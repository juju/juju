// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
)

// NewTryInOrderLoginProvider returns a login provider that will attempt to
// log in using all the specified login providers in sequence - results
// of the first on that succeeds will be returned.
// This login provider should only be used when connecting to a controller
// for the first time when we still don't know which login method.
func NewTryInOrderLoginProvider(logger loggo.Logger, providers ...LoginProvider) LoginProvider {
	return &tryInOrderLoginProviders{
		providers:  providers,
		logger:     logger,
		authHeader: missingHeader,
	}
}

func missingHeader() (http.Header, error) {
	return nil, ErrorLoginFirst
}

type tryInOrderLoginProviders struct {
	providers  []LoginProvider
	logger     loggo.Logger
	authHeader func() (http.Header, error)
}

// AuthHeader implements the [LoginProvider.AuthHeader] method.
// It attempts to retrieve the auth header from the last successful login provider.
// If login was never attempted/successful, an ErrorLoginFirst error is returned.
func (p *tryInOrderLoginProviders) AuthHeader() (http.Header, error) {
	return p.authHeader()
}

// Login implements the LoginProvider.Login method.
func (p *tryInOrderLoginProviders) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var lastError error
	for i, provider := range p.providers {
		result, err := provider.Login(ctx, caller)
		if err != nil {
			p.logger.Debugf("login error using provider %d - %s", i, err.Error())
		} else {
			p.logger.Debugf("successful login using provider %d", i)
			p.authHeader = func() (http.Header, error) { return provider.AuthHeader() }
			return result, nil
		}
		lastError = err
	}
	return nil, errors.Trace(lastError)
}
