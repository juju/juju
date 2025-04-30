// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
)

// DefaultProvider exposes the various common implementations found in
// this package as methods of a single type. This facilitates treating
// the implementations as a bundle, e.g. satisfying interfaces.
type DefaultProvider struct {
	// Env is the Juju environment that methods target.
	Env environs.Environ
}

// BootstrapEnv bootstraps the Juju environment.
func (dp DefaultProvider) BootstrapEnv(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	result, err := Bootstrap(ctx, dp.Env, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// DestroyEnv destroys the Juju environment.
func (dp DefaultProvider) DestroyEnv(ctx context.Context) error {
	if err := Destroy(dp.Env, ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
