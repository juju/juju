// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/caas"
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

	// NewContainerBroker is a func that returns a caas container broker
	// for the relevant model.
	NewContainerBroker caas.NewContainerBrokerFunc
}

// CloudAPIVersion returns the cloud API version for the cloud with the given spec.
func (g EnvironConfigGetter) CloudAPIVersion(spec environs.CloudSpec) (string, error) {
	// Only CAAS models have an API version we care about right now.
	if g.Model.Type() == state.ModelTypeIAAS {
		return "", nil
	}
	cfg, err := g.ModelConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	ctrlCfg, err := g.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	newBroker := g.NewContainerBroker
	if newBroker == nil {
		newBroker = caas.New
	}
	broker, err := newBroker(environs.OpenParams{
		ControllerUUID: ctrlCfg.ControllerUUID(),
		Cloud:          spec,
		Config:         cfg,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return broker.APIVersion()
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
		cloudCredential := cloud.NewNamedCredential(credentialValue.Name,
			cloud.AuthType(credentialValue.AuthType),
			credentialValue.Attributes,
			credentialValue.Revoked,
		)
		credential = &cloudCredential
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
		g := EnvironConfigGetter{State: st, Model: m}
		return environs.GetEnviron(g, newEnviron)
	}
}

// NewCAASBrokerFunc defines the type of a function that, given a state.State,
// returns a new CAAS broker.
type NewCAASBrokerFunc func(*state.State) (caas.Broker, error)

// GetNewCAASBrokerFunc returns a NewCAASBrokerFunc, that constructs CAAS brokers
// using the given caas.NewContainerBrokerFunc.
func GetNewCAASBrokerFunc(newBroker caas.NewContainerBrokerFunc) NewCAASBrokerFunc {
	return func(st *state.State) (caas.Broker, error) {
		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		g := EnvironConfigGetter{State: st, Model: m}
		cloudSpec, err := g.CloudSpec()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err := g.ModelConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ctrlCfg, err := g.ControllerConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newBroker(environs.OpenParams{
			ControllerUUID: ctrlCfg.ControllerUUID(),
			Cloud:          cloudSpec,
			Config:         cfg,
		})
	}
}
