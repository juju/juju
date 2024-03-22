// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

// NewTryInOrderLoginProvider returns a login provider that will attempt to
// log in using all the specified login providers in sequence - results
// of the first on that succeeds will be returned.
// This login provider should only be used when connecting to a controller
// for the first time when we still don't know which login method.
func NewTryInOrderLoginProvider(providers ...LoginProvider) LoginProvider {
	return &tryInOrderLoginProviders{
		providers: providers,
	}
}

type tryInOrderLoginProviders struct {
	providers []LoginProvider
}

// Login implements the LoginProvider.Login method.
func (p *tryInOrderLoginProviders) Login(ctx context.Context, caller base.APICaller) (*LoginResultParams, error) {
	var lastError error
	for _, provider := range p.providers {
		result, err := provider.Login(ctx, caller)
		if err == nil {
			return result, nil
		}
		lastError = err
	}
	return nil, errors.Trace(lastError)
}
