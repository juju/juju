// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// EnvironConfigGetterFuncs holds implements environs.EnvironConfigGetter
// in a pluggable way.
type EnvironConfigGetterFuncs struct {
	ModelConfigFunc func() (*config.Config, error)
	CloudSpecFunc   func() (environscloudspec.CloudSpec, error)
}

// ModelConfig implements EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ModelConfig(ctx context.Context) (*config.Config, error) {
	return f.ModelConfigFunc()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return f.CloudSpecFunc()
}

// NewEnvironFunc is a function that returns a BootstrapEnviron instance.
type NewEnvironFunc func(context.Context) (environs.BootstrapEnviron, error)

// EnvironFuncForModel is a helper function that returns a NewEnvironFunc suitable for
// the specified model.
func EnvironFuncForModel(model stateenvirons.Model, credentialService stateenvirons.CredentialService, configGetter environs.EnvironConfigGetter) NewEnvironFunc {
	if model.Type() == state.ModelTypeCAAS {
		return func(ctx context.Context) (environs.BootstrapEnviron, error) {
			f := stateenvirons.GetNewCAASBrokerFunc(caas.New)
			return f(model, credentialService)
		}
	}
	return func(ctx context.Context) (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(ctx, configGetter, environs.New)
	}
}
