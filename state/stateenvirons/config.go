// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

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
func (g EnvironConfigGetter) CloudSpec(tag names.ModelTag) (environs.CloudSpec, error) {
	model, err := g.GetModel(tag)
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}
	cloudName := model.Cloud()
	regionName := model.CloudRegion()
	credentialName := model.CloudCredential()
	modelOwner := model.Owner()
	return CloudSpec(g.State, cloudName, regionName, credentialName, modelOwner)
}

// CloudSpec returns an environs.CloudSpec from a *state.State,
// given the cloud, region and credential names.
func CloudSpec(
	accessor state.CloudAccessor,
	cloudName, regionName, credentialName string,
	credentialOwner names.UserTag,
) (environs.CloudSpec, error) {
	modelCloud, err := accessor.Cloud(cloudName)
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}

	var credential *cloud.Credential
	if credentialName != "" {
		credentials, err := accessor.CloudCredentials(credentialOwner, cloudName)
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

	return environs.MakeCloudSpec(modelCloud, cloudName, regionName, credential)
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
