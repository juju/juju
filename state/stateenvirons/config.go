// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type baseModel interface {
	Cloud() (cloud.Cloud, error)
	CloudRegion() string
	CloudCredential() (state.Credential, bool, error)
}

// Model exposes the methods needed for an EnvironConfigGetter.
type Model interface {
	baseModel
	ModelTag() names.ModelTag
	ControllerUUID() string
	Type() state.ModelType
	Config() (*config.Config, error)
}

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a Model.
type EnvironConfigGetter struct {
	Model

	// NewContainerBroker is a func that returns a caas container broker
	// for the relevant model.
	NewContainerBroker caas.NewContainerBrokerFunc
}

// CloudAPIVersion returns the cloud API version for the cloud with the given spec.
func (g EnvironConfigGetter) CloudAPIVersion(spec environscloudspec.CloudSpec) (string, error) {
	// Only CAAS models have an API version we care about right now.
	if g.Model.Type() == state.ModelTypeIAAS {
		return "", nil
	}
	cfg, err := g.Config()
	if err != nil {
		return "", errors.Trace(err)
	}
	newBroker := g.NewContainerBroker
	if newBroker == nil {
		newBroker = caas.New
	}
	broker, err := newBroker(context.TODO(), environs.OpenParams{
		ControllerUUID: g.Model.ControllerUUID(),
		Cloud:          spec,
		Config:         cfg,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return broker.APIVersion()
}

// ModelConfig implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) ModelConfig() (*config.Config, error) {
	return g.Config()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) CloudSpec() (environscloudspec.CloudSpec, error) {
	return CloudSpecForModel(g.Model)
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(m baseModel) (environscloudspec.CloudSpec, error) {
	cloud, err := m.Cloud()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := m.CloudRegion()
	credentialValue, ok, err := m.CloudCredential()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	var credential *state.Credential
	if ok {
		credential = &credentialValue
	}
	return CloudSpec(cloud, regionName, credential)
}

// CloudSpec returns an environscloudspec.CloudSpec from a *state.State,
// given the cloud, region and credential names.
func CloudSpec(
	modelCloud cloud.Cloud,
	regionName string,
	credential *state.Credential,
) (environscloudspec.CloudSpec, error) {
	var cloudCredential *cloud.Credential
	if credential != nil {
		cloudCredentialValue := cloud.NewNamedCredential(credential.Name,
			cloud.AuthType(credential.AuthType),
			credential.Attributes,
			credential.Revoked,
		)
		cloudCredential = &cloudCredentialValue
	}

	return environscloudspec.MakeCloudSpec(modelCloud, regionName, cloudCredential)
}

// NewEnvironFunc aliases a function that, given a Model,
// returns a new Environ.
type NewEnvironFunc = func(Model) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(m Model) (environs.Environ, error) {
		g := EnvironConfigGetter{Model: m}
		return environs.GetEnviron(g, newEnviron)
	}
}

// NewCAASBrokerFunc aliases a function that, given a state.State,
// returns a new CAAS broker.
type NewCAASBrokerFunc = func(Model) (caas.Broker, error)

// GetNewCAASBrokerFunc returns a NewCAASBrokerFunc, that constructs CAAS brokers
// using the given caas.NewContainerBrokerFunc.
func GetNewCAASBrokerFunc(newBroker caas.NewContainerBrokerFunc) NewCAASBrokerFunc {
	return func(m Model) (caas.Broker, error) {
		g := EnvironConfigGetter{Model: m}
		cloudSpec, err := g.CloudSpec()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err := g.Config()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newBroker(context.TODO(), environs.OpenParams{
			ControllerUUID: m.ControllerUUID(),
			Cloud:          cloudSpec,
			Config:         cfg,
		})
	}
}
