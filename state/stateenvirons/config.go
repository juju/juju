// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a *state.State.
type EnvironConfigGetter struct {
	*state.State
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) CloudSpec() (environs.CloudSpec, error) {
	model, err := g.Model()
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}

	cloudName := model.Cloud()
	regionName := model.CloudRegion()
	credentialName := model.CloudCredential()
	modelOwner := model.Owner()

	modelCloud, err := g.Cloud(cloudName)
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}
	if regionName != "" {
		region, err := cloud.RegionByName(modelCloud.Regions, regionName)
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		modelCloud.Endpoint = region.Endpoint
		modelCloud.StorageEndpoint = region.StorageEndpoint
	}

	var credential *cloud.Credential
	if credentialName != "" {
		credentials, err := g.CloudCredentials(modelOwner, cloudName)
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		var ok bool
		credentialValue, ok := credentials[credentialName]
		if !ok {
			return environs.CloudSpec{}, errors.NotFoundf("credential %q", credentialName)
		}
		credential = &credentialValue
	}

	return environs.CloudSpec{
		modelCloud.Type,
		cloudName,
		regionName,
		modelCloud.Endpoint,
		modelCloud.StorageEndpoint,
		credential,
	}, nil
}

// NewEnvironFunc defines the type of a function that, given a state.State,
// returns a new Environ.
type NewEnvironFunc func(*state.State) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(st *state.State) (environs.Environ, error) {
		g := EnvironConfigGetter{st}
		return environs.GetEnviron(g, newEnviron)
	}
}
