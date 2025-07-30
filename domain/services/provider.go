// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
	modelservice "github.com/juju/juju/domain/model/service"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	statemodel "github.com/juju/juju/domain/model/state/model"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modelconfigstate "github.com/juju/juju/domain/modelconfig/state"
)

// ProviderServices provides access to the services required by the apiserver.
type ProviderServices struct {
	modelServiceFactoryBase
}

// NewProviderServices returns a new registry which uses the provided db
// function to obtain a model database.
func NewProviderServices(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *ProviderServices {
	return &ProviderServices{
		modelServiceFactoryBase: modelServiceFactoryBase{
			serviceFactoryBase: serviceFactoryBase{
				controllerDB: controllerDB,
				logger:       logger,
			},
			modelDB: modelDB,
		},
	}
}

// Model returns the provider model service.
func (s *ProviderServices) Model() *modelservice.ProviderService {
	return modelservice.NewProviderService(
		statecontroller.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
		),
		statemodel.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("modelinfo"),
		),
		s.controllerWatcherFactory("model"),
	)
}

// Cloud returns the provider cloud service.
func (s *ProviderServices) Cloud() *cloudservice.WatchableProviderService {
	return cloudservice.NewWatchableProviderService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("cloud"),
	)
}

// Credential returns the provider credential service.
func (s *ProviderServices) Credential() *credentialservice.WatchableProviderService {
	return credentialservice.NewWatchableProviderService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("credential"),
	)
}

// Config returns the provider model config service.
func (s *ProviderServices) Config() *modelconfigservice.WatchableProviderService {
	return modelconfigservice.NewWatchableProviderService(
		modelconfigstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("modelconfig"),
	)
}
