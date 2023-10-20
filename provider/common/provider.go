// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

// DefaultProvider exposes the various common implementations found in
// this package as methods of a single type. This facilitates treating
// the implementations as a bundle, e.g. satisfying interfaces.
type DefaultProvider struct {
	// Env is the Juju environment that methods target.
	Env environs.Environ
}

// BootstrapEnv bootstraps the Juju environment.
func (dp DefaultProvider) BootstrapEnv(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	result, err := Bootstrap(ctx, dp.Env, callCtx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// DestroyEnv destroys the Juju environment.
func (dp DefaultProvider) DestroyEnv(ctx envcontext.ProviderCallContext) error {
	if err := Destroy(dp.Env, ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
