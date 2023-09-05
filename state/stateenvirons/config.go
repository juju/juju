// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

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
	CloudCredentialTag() (names.CloudCredentialTag, bool)
}

// Model exposes the methods needed for an EnvironConfigGetter.
type Model interface {
	baseModel
	ModelTag() names.ModelTag
	ControllerUUID() string
	Type() state.ModelType
	Config() (*config.Config, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, tag names.CloudCredentialTag) (cloud.Credential, error)
}

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a Model.
type EnvironConfigGetter struct {
	Model

	// NewContainerBroker is a func that returns a caas container broker
	// for the relevant model.
	NewContainerBroker caas.NewContainerBrokerFunc

	// CredentialService provides access to credentials.
	CredentialService CredentialService
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
func (g EnvironConfigGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.Config()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return CloudSpecForModel(ctx, g.Model, g.CredentialService)
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(ctx context.Context, m baseModel, credentialService CredentialService) (environscloudspec.CloudSpec, error) {
	cld, err := m.Cloud()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := m.CloudRegion()
	tag, ok := m.CloudCredentialTag()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	var credential cloud.Credential
	if ok {
		credential, err = credentialService.CloudCredential(ctx, tag)
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}
	}
	return environscloudspec.MakeCloudSpec(cld, regionName, &credential)
}

// NewEnvironFunc aliases a function that, given a Model,
// returns a new Environ.
type NewEnvironFunc = func(Model, CredentialService) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(m Model, credentialService CredentialService) (environs.Environ, error) {
		g := EnvironConfigGetter{Model: m, CredentialService: credentialService}
		return environs.GetEnviron(context.TODO(), g, newEnviron)
	}
}

// NewCAASBrokerFunc aliases a function that, given a state.State,
// returns a new CAAS broker.
type NewCAASBrokerFunc = func(Model, CredentialService) (caas.Broker, error)

// GetNewCAASBrokerFunc returns a NewCAASBrokerFunc, that constructs CAAS brokers
// using the given caas.NewContainerBrokerFunc.
func GetNewCAASBrokerFunc(newBroker caas.NewContainerBrokerFunc) NewCAASBrokerFunc {
	return func(m Model, credentialService CredentialService) (caas.Broker, error) {
		g := EnvironConfigGetter{Model: m, CredentialService: credentialService}
		cloudSpec, err := g.CloudSpec(context.TODO())
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
