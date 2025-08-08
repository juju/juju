// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	agentbinarystate "github.com/juju/juju/domain/agentbinary/state"
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
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modeldefaultsstate "github.com/juju/juju/domain/modeldefaults/state"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	upgradestate "github.com/juju/juju/domain/upgrade/state"
	"github.com/juju/juju/internal/errors"
)

// ControllerServices provides access to the services required by the apiserver.
type ControllerServices struct {
	serviceFactoryBase

	controllerObjectStore objectstore.NamespacedObjectStoreGetter
	clock                 clock.Clock
	loggerContextGetter   logger.LoggerContextGetter
}

// NewControllerServices returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewControllerServices(
	controllerDB changestream.WatchableDBFactory,
	controllerObjectStoreGetter objectstore.NamespacedObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
	loggerContextGetter logger.LoggerContextGetter,
) *ControllerServices {
	return &ControllerServices{
		serviceFactoryBase: serviceFactoryBase{
			controllerDB: controllerDB,
			logger:       logger,
		},
		controllerObjectStore: controllerObjectStoreGetter,
		clock:                 clock,
		loggerContextGetter:   loggerContextGetter,
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
		s.controllerWatcherFactory("controllerconfig"),
	)
}

// ControllerNode returns the controller node service.
func (s *ControllerServices) ControllerNode() *controllernodeservice.WatchableService {
	return controllernodeservice.NewWatchableService(
		controllernodestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("controllernode"),
		s.logger.Child("controllernode"),
	)
}

// Model returns the model service.
func (s *ControllerServices) Model() *modelservice.WatchableService {
	logger := s.logger.Child("model")
	return modelservice.NewWatchableService(
		statecontroller.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("model"),
		statusHistoryGetter{
			loggerContextGetter: s.loggerContextGetter,
			clock:               s.clock,
		},
		s.clock,
		logger,
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
		s.controllerWatcherFactory("externalcontroller"),
	)
}

// Credential returns the credential service.
func (s *ControllerServices) Credential() *credentialservice.WatchableService {
	return credentialservice.NewWatchableService(
		credentialstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("credential"),
		s.logger.Child("credential"),
	)
}

// Cloud returns the cloud service.
func (s *ControllerServices) Cloud() *cloudservice.WatchableService {
	return cloudservice.NewWatchableService(
		cloudstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("cloud"),
	)
}

// AutocertCache returns the autocert cache service.
func (s *ControllerServices) AutocertCache() *autocertcacheservice.Service {
	return autocertcacheservice.NewService(
		autocertcachestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.logger.Child("autocertcache"),
	)
}

// Upgrade returns the upgrade service.
func (s *ControllerServices) Upgrade() *upgradeservice.WatchableService {
	return upgradeservice.NewWatchableService(
		upgradestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("upgrade"),
	)
}

// Flag returns the flag service.
func (s *ControllerServices) Flag() *flagservice.Service {
	return flagservice.NewService(
		flagstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.logger.Child("flag")),
	)
}

// Access returns the access service, this includes users and permissions.
func (s *ControllerServices) Access() *accessservice.Service {
	return accessservice.NewService(
		accessstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.logger.Child("access")),
	)
}

func (s *ControllerServices) SecretBackend() *secretbackendservice.WatchableService {
	log := s.logger.Child("secretbackend")

	return secretbackendservice.NewWatchableService(
		secretbackendstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), log),
		log,
		s.controllerWatcherFactory("secretbackend"),
	)
}

func (s *ControllerServices) Macaroon() *macaroonservice.Service {
	return macaroonservice.NewService(
		macaroonstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.clock,
	)
}

// ControllerAgentBinaryStore returns the [agentbinaryservice.AgentBinaryStore]
// for the entire controller. This should be used when wanting to cache agent
// binaries controller wide.
func (s *ControllerServices) ControllerAgentBinaryStore() *agentbinaryservice.AgentBinaryStore {
	return agentbinaryservice.NewAgentBinaryStore(
		agentbinarystate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.logger.Child("agentbinary"),
		s.controllerObjectStore,
	)
}

type statusHistoryGetter struct {
	loggerContextGetter logger.LoggerContextGetter
	clock               clock.Clock
}

// GetLoggerContext returns a logger context for the given model UUID.
func (l statusHistoryGetter) GetStatusHistoryForModel(ctx context.Context, modelUUID model.UUID) (modelservice.StatusHistory, error) {
	loggerContext, err := l.loggerContextGetter.GetLoggerContext(ctx, modelUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	logger := loggerContext.GetLogger("juju.services")
	return domain.NewStatusHistory(logger, l.clock), nil
}
