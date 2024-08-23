// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
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
	modelagentservice "github.com/juju/juju/domain/modelagent/service"
	modelagentstate "github.com/juju/juju/domain/modelagent/state"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modeldefaultsstate "github.com/juju/juju/domain/modeldefaults/state"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	upgradestate "github.com/juju/juju/domain/upgrade/state"
)

// ControllerFactory provides access to the services required by the apiserver.
type ControllerFactory struct {
	controllerDB changestream.WatchableDBFactory
	dbDeleter    database.DBDeleter
	logger       logger.Logger
}

// NewControllerFactory returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewControllerFactory(
	controllerDB changestream.WatchableDBFactory,
	dbDeleter database.DBDeleter,
	logger logger.Logger,
) *ControllerFactory {
	return &ControllerFactory{
		controllerDB: controllerDB,
		dbDeleter:    dbDeleter,
		logger:       logger,
	}
}

// Controller returns the controller service.
func (s *ControllerFactory) Controller() *controllerservice.Service {
	return controllerservice.NewService(
		controllerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ControllerConfig returns the controller configuration service.
func (s *ControllerFactory) ControllerConfig() *controllerconfigservice.WatchableService {
	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("controllerconfig"),
		),
	)
}

// ControllerNode returns the controller node service.
func (s *ControllerFactory) ControllerNode() *controllernodeservice.Service {
	return controllernodeservice.NewService(
		controllernodestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// Model returns the model service.
func (s *ControllerFactory) Model() *modelservice.Service {
	return modelservice.NewService(
		modelstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.dbDeleter,
		modelservice.DefaultAgentBinaryFinder(),
		s.logger,
	)
}

// ModelDefaults returns the model defaults service.
func (s *ControllerFactory) ModelDefaults() *modeldefaultsservice.Service {
	return modeldefaultsservice.NewService(
		modeldefaultsstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ExternalController returns the external controller service.
func (s *ControllerFactory) ExternalController() *externalcontrollerservice.WatchableService {
	return externalcontrollerservice.NewWatchableService(
		externalcontrollerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("externalcontroller"),
		),
	)
}

// Credential returns the credential service.
func (s *ControllerFactory) Credential() *credentialservice.WatchableService {
	return credentialservice.NewWatchableService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("credential"),
		),
		s.logger,
	)
}

// Cloud returns the cloud service.
func (s *ControllerFactory) Cloud() *cloudservice.WatchableService {
	return cloudservice.NewWatchableService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("cloud"),
		),
	)
}

// AutocertCache returns the autocert cache service.
func (s *ControllerFactory) AutocertCache() *autocertcacheservice.Service {
	return autocertcacheservice.NewService(
		autocertcachestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.logger.Child("autocertcache"),
	)
}

// Upgrade returns the upgrade service.
func (s *ControllerFactory) Upgrade() *upgradeservice.WatchableService {
	return upgradeservice.NewWatchableService(
		upgradestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("upgrade"),
		),
	)
}

// AgentObjectStore returns the object store service.
func (s *ControllerFactory) AgentObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("objectstore"),
		),
	)
}

// Flag returns the flag service.
func (s *ControllerFactory) Flag() *flagservice.Service {
	return flagservice.NewService(
		flagstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.logger.Child("flag")),
	)
}

// Access returns the access service, this includes users and permissions.
func (s *ControllerFactory) Access() *accessservice.Service {
	return accessservice.NewService(
		accessstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.logger.Child("access")),
	)
}

func (s *ControllerFactory) SecretBackend() *secretbackendservice.WatchableService {
	logger := s.logger.Child("secretbackend")
	state := secretbackendstate.NewState(
		changestream.NewTxnRunnerFactory(s.controllerDB),
		logger.Child("state"),
	)
	return secretbackendservice.NewWatchableService(
		state,
		logger.Child("service"),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("watcherfactory"),
		),
	)
}

func (s *ControllerFactory) Macaroon() *macaroonservice.Service {
	return macaroonservice.NewService(
		macaroonstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		clock.WallClock,
	)
}

// Agent returns the model agent service.
func (s *ControllerFactory) Agent() *modelagentservice.Service {
	return modelagentservice.NewService(
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}
