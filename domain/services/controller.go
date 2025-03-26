// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	autocertcachestate "github.com/juju/juju/domain/autocert/state"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	controllerservice "github.com/juju/juju/domain/controller/service"
	controllerstate "github.com/juju/juju/domain/controller/state"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	controllernodestate "github.com/juju/juju/domain/controllernode/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	externalcontrollerstate "github.com/juju/juju/domain/externalcontroller/state"
	flagservice "github.com/juju/juju/domain/flag/service"
	flagstate "github.com/juju/juju/domain/flag/state"
	macaroonservice "github.com/juju/juju/domain/macaroon/service"
	macaroonstate "github.com/juju/juju/domain/macaroon/state"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modeldefaultsstate "github.com/juju/juju/domain/modeldefaults/state"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	upgradestate "github.com/juju/juju/domain/upgrade/state"
)

// ControllerServices provides access to the services required by the apiserver.
type ControllerServices struct {
	serviceFactoryBase

	dbDeleter database.DBDeleter
	clock     clock.Clock
}

// NewControllerServices returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewControllerServices(
	controllerDB changestream.WatchableDBFactory,
	dbDeleter database.DBDeleter,
	objectStore objectstore.ModelObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) *ControllerServices {
	return &ControllerServices{
		serviceFactoryBase: serviceFactoryBase{
			controllerDB: controllerDB,
			logger:       logger,
		},
		dbDeleter: dbDeleter,
		clock:     clock,
	}
}

// Controller returns the controller service.
func (s *ControllerServices) Controller() *controllerservice.Service {
	return controllerservice.NewService(
		controllerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ControllerConfig returns the controller configuration service.
func (s *ControllerServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(),
	)
}

// ControllerNode returns the controller node service.
func (s *ControllerServices) ControllerNode() *controllernodeservice.Service {
	return controllernodeservice.NewService(
		controllernodestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// Model returns the model service.
func (s *ControllerServices) Model() *modelservice.WatchableService {
	return modelservice.NewWatchableService(
		modelstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.dbDeleter,
		s.loggerFor("model"),
		s.controllerWatcherFactory(),
	)
}

// ModelDefaults returns the model defaults service.
func (s *ControllerServices) ModelDefaults() *modeldefaultsservice.Service {
	return modeldefaultsservice.NewService(
		modeldefaultsservice.ProviderModelConfigGetter(),
		modeldefaultsstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ExternalController returns the external controller service.
func (s *ControllerServices) ExternalController() *externalcontrollerservice.WatchableService {
	return externalcontrollerservice.NewWatchableService(
		externalcontrollerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(),
	)
}

// Credential returns the credential service.
func (s *ControllerServices) Credential() *credentialservice.WatchableService {
	return credentialservice.NewWatchableService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(),
		s.loggerFor("credential"),
	)
}

// Cloud returns the cloud service.
func (s *ControllerServices) Cloud() *cloudservice.WatchableService {
	return cloudservice.NewWatchableService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(),
	)
}

// AutocertCache returns the autocert cache service.
func (s *ControllerServices) AutocertCache() *autocertcacheservice.Service {
	return autocertcacheservice.NewService(
		autocertcachestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.loggerFor("autocert"),
	)
}

// Upgrade returns the upgrade service.
func (s *ControllerServices) Upgrade() *upgradeservice.WatchableService {
	return upgradeservice.NewWatchableService(
		upgradestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(),
	)
}

// Flag returns the flag service.
func (s *ControllerServices) Flag() *flagservice.Service {
	return flagservice.NewService(
		flagstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.loggerFor("flag")),
	)
}

// Access returns the access service, this includes users and permissions.
func (s *ControllerServices) Access() *accessservice.Service {
	return accessservice.NewService(
		accessstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.loggerFor("access")),
	)
}

func (s *ControllerServices) SecretBackend() *secretbackendservice.WatchableService {
	logger := s.loggerFor("secretbackend")

	return secretbackendservice.NewWatchableService(
		secretbackendstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), logger),
		logger,
		s.controllerWatcherFactory(),
	)
}

func (s *ControllerServices) Macaroon() *macaroonservice.Service {
	return macaroonservice.NewService(
		macaroonstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.clock,
	)
}
