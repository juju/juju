// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modelconfigstate "github.com/juju/juju/domain/modelconfig/state"
)

// ProviderFactory provides access to the services required by the apiserver.
type ProviderFactory struct {
	logger       logger.Logger
	controllerDB changestream.WatchableDBFactory
	modelDB      changestream.WatchableDBFactory
}

// NewProviderFactory returns a new registry which uses the provided db
// function to obtain a model database.
func NewProviderFactory(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *ProviderFactory {
	return &ProviderFactory{
		logger:       logger,
		controllerDB: controllerDB,
		modelDB:      modelDB,
	}
}

// Model returns the provider model service.
func (s *ProviderFactory) Model() *modelservice.ProviderService {
	return modelservice.NewProviderService(
		modelstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// Cloud returns the provider cloud service.
func (s *ProviderFactory) Cloud() *cloudservice.WatchableProviderService {
	return cloudservice.NewWatchableProviderService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("cloud"),
		),
	)
}

// Credential returns the provider credential service.
func (s *ProviderFactory) Credential() *credentialservice.WatchableProviderService {
	return credentialservice.NewWatchableProviderService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("credential"),
		),
	)
}

// Config returns the provider model config service.
func (s *ProviderFactory) Config() *modelconfigservice.WatchableProviderService {
	return modelconfigservice.NewWatchableProviderService(
		modelconfigstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("modelconfig")),
	)
}
