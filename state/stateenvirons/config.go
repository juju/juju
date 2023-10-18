// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type baseModel interface {
	CloudName() string
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
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	Get(ctx context.Context, name string) (*cloud.Cloud, error)
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

	// CloudService provides access to clouds.
	CloudService CloudService
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
	return CloudSpecForModel(ctx, g.Model, g.CloudService, g.CredentialService)
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(
	ctx context.Context, m baseModel,
	cloudService CloudService,
	credentialService CredentialService,
) (environscloudspec.CloudSpec, error) {
	cld, err := cloudService.Get(ctx, m.CloudName())
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := m.CloudRegion()
	tag, ok := m.CloudCredentialTag()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	var cred cloud.Credential
	if ok {
		cred, err = credentialService.CloudCredential(ctx, credential.IdFromTag(tag))
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}
	}
	return environscloudspec.MakeCloudSpec(*cld, regionName, &cred)
}

// NewEnvironFunc aliases a function that, given a Model,
// returns a new Environ.
type NewEnvironFunc = func(Model, CloudService, CredentialService) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(m Model, cloudService CloudService, credentialService CredentialService) (environs.Environ, error) {
		g := EnvironConfigGetter{Model: m, CloudService: cloudService, CredentialService: credentialService}
		return environs.GetEnviron(context.TODO(), g, newEnviron)
	}
}

// NewCAASBrokerFunc aliases a function that, given a state.State,
// returns a new CAAS broker.
type NewCAASBrokerFunc = func(Model, CloudService, CredentialService) (caas.Broker, error)

// GetNewCAASBrokerFunc returns a NewCAASBrokerFunc, that constructs CAAS brokers
// using the given caas.NewContainerBrokerFunc.
func GetNewCAASBrokerFunc(newBroker caas.NewContainerBrokerFunc) NewCAASBrokerFunc {
	return func(m Model, cloudService CloudService, credentialService CredentialService) (caas.Broker, error) {
		g := EnvironConfigGetter{Model: m, CloudService: cloudService, CredentialService: credentialService}
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
