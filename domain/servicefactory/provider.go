// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
)

// ProviderFactory provides access to the services required by the apiserver.
type ProviderFactory struct {
	logger       Logger
	controllerDB changestream.WatchableDBFactory
	modelDB      changestream.WatchableDBFactory
}

// NewProviderFactory returns a new registry which uses the provided db
// function to obtain a model database.
func NewProviderFactory(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	logger Logger,
) *ProviderFactory {
	return &ProviderFactory{
		logger:       logger,
		controllerDB: controllerDB,
		modelDB:      modelDB,
	}
}

// Cloud returns the cloud service.
func (s *ProviderFactory) Cloud() *cloudservice.WatchableProviderService {
	return cloudservice.NewWatchableProviderService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("cloud"),
		),
	)
}

// Credential returns the credential service.
func (s *ProviderFactory) Credential() *credentialservice.WatchableProviderService {
	return credentialservice.NewWatchableProviderService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("credential"),
		),
	)
}
