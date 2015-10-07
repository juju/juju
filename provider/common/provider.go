// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
)

// DefaultProvider exposes the various common implementations found in
// this package as methods of a single type. This facilitates treating
// the implentations as a bundle, e.g. satisfying interfaces.
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
func (dp DefaultProvider) DestroyEnv() error {
	if err := Destroy(dp.Env); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SupportedArchitectures returns all the image architectures for env
// matching the constraints.
func (dp DefaultProvider) SupportedArchitectures(imageConstraint *imagemetadata.ImageConstraint) ([]string, error) {
	arches, err := SupportedArchitectures(dp.Env, imageConstraint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return arches, nil
}
