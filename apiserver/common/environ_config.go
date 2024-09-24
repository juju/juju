// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// NewEnvironFunc is a function that returns a BootstrapEnviron instance.
type NewEnvironFunc func(context.Context) (environs.BootstrapEnviron, error)

// EnvironFuncForModel is a helper function that returns a NewEnvironFunc suitable for
// the specified model.
func EnvironFuncForModel(model stateenvirons.Model, cloudService CloudService,
	credentialService stateenvirons.CredentialService,
	configGetter environs.EnvironConfigGetter,
) NewEnvironFunc {
	if model.Type() == state.ModelTypeCAAS {
		return func(ctx context.Context) (environs.BootstrapEnviron, error) {
			f := stateenvirons.GetNewCAASBrokerFunc(caas.New)
			return f(model, cloudService, credentialService, configGetter)
		}
	}
	return func(ctx context.Context) (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(ctx, configGetter, environs.New)
	}
}
