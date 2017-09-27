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
// TODO - CAAS(externalreality): Once cloud methods are migrated
// to model EnvironConfigGetter will no longer need to contain
// both state and model but only model.
type EnvironConfigGetter struct {
	*state.State
	*state.Model
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) CloudSpec() (environs.CloudSpec, error) {
	cloudName := g.Model.Cloud()
	regionName := g.Model.CloudRegion()
	credentialTag, _ := g.Model.CloudCredential()
	return CloudSpec(g.State, cloudName, regionName, credentialTag)
}

// CloudSpec returns an environs.CloudSpec from a *state.State,
// given the cloud, region and credential names.
func CloudSpec(
	accessor state.CloudAccessor,
	cloudName, regionName string,
	credentialTag names.CloudCredentialTag,
) (environs.CloudSpec, error) {
	modelCloud, err := accessor.Cloud(cloudName)
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}

	var credential *cloud.Credential
	if credentialTag != (names.CloudCredentialTag{}) {
		credentialValue, err := accessor.CloudCredential(credentialTag)
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		credential = &credentialValue
	}

	return environs.MakeCloudSpec(modelCloud, regionName, credential)
}

// NewEnvironFunc defines the type of a function that, given a state.State,
// returns a new Environ.
type NewEnvironFunc func(*state.State) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(st *state.State) (environs.Environ, error) {
		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		g := EnvironConfigGetter{st, m}
		return environs.GetEnviron(g, newEnviron)
	}
}
