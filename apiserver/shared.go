// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/watcherregistry"
)

// sharedServerContext contains a number of components that are unchangeable in the API server.
// These components need to be exposed through the facade.ModelContext. Instead of having the methods
// of newAPIHandler and newAPIRoot take ever-increasing numbers of parameters, they will instead
// have a pointer to the sharedServerContext.
//
// All attributes in the context should be goroutine aware themselves, like the state pool, hub, and
// presence, or protected and only accessed through methods on this context object.
type sharedServerContext struct {
	leaseManager       lease.Manager
	logger             corelogger.Logger
	charmhubHTTPClient facade.HTTPClient

	// dbGetter is used to access databases from the API server. Along with
	// creating a new database for new models and during model migrations.
	dbGetter changestream.WatchableDBGetter

	// dbDeleter is used to delete the database when a model migration fails
	// and the model is being removed.
	dbDeleter database.DBDeleter

	// DomainServicesGetter is used to get the domain services for controllers
	// and models.
	domainServicesGetter    services.DomainServicesGetter
	controllerConfigService ControllerConfigService

	// TraceGetter is used to get the tracer for the API server.
	tracerGetter trace.TracerGetter

	// ObjectStoreGetter is used to get the object store for storing blobs
	// for the API server.
	objectStoreGetter objectstore.ObjectStoreGetter

	// watcherRegistryGetter is used to get the watcher registry for the API
	// server.
	watcherRegistryGetter watcherregistry.WatcherRegistryGetter

	configMutex sync.RWMutex

	// controllerUUID is the unique identifier of the controller.
	controllerUUID   string
	controllerConfig controller.Config
	features         set.Strings

	// controllerModelUUID is the UUID of the controller model.
	controllerModelUUID model.UUID

	machineTag names.Tag
	dataDir    string
	logDir     string
}

type sharedServerConfig struct {
	leaseManager        lease.Manager
	controllerUUID      string
	controllerModelUUID model.UUID
	controllerConfig    controller.Config
	logger              corelogger.Logger
	charmhubHTTPClient  facade.HTTPClient

	dbGetter                changestream.WatchableDBGetter
	dbDeleter               database.DBDeleter
	domainServicesGetter    services.DomainServicesGetter
	controllerConfigService ControllerConfigService
	tracerGetter            trace.TracerGetter
	objectStoreGetter       objectstore.ObjectStoreGetter
	watcherRegistryGetter   watcherregistry.WatcherRegistryGetter
	machineTag              names.Tag
	dataDir                 string
	logDir                  string
}

func (c *sharedServerConfig) validate() error {
	if c.leaseManager == nil {
		return errors.NotValidf("nil leaseManager")
	}
	if c.controllerUUID == "" {
		return errors.NotValidf("empty controllerUUID")
	}
	if c.controllerConfig == nil {
		return errors.NotValidf("nil controllerConfig")
	}
	if c.dbGetter == nil {
		return errors.NotValidf("nil dbGetter")
	}
	if c.dbDeleter == nil {
		return errors.NotValidf("nil dbDeleter")
	}
	if c.domainServicesGetter == nil {
		return errors.NotValidf("nil domainServicesGetter")
	}
	if c.controllerConfigService == nil {
		return errors.NotValidf("nil controllerConfigService")
	}
	if c.tracerGetter == nil {
		return errors.NotValidf("nil tracerGetter")
	}
	if c.objectStoreGetter == nil {
		return errors.NotValidf("nil objectStoreGetter")
	}
	if c.watcherRegistryGetter == nil {
		return errors.NotValidf("nil watcherRegistryGetter")
	}
	if c.machineTag == nil {
		return errors.NotValidf("empty machineTag")
	}
	return nil
}

func newSharedServerContext(config sharedServerConfig) (*sharedServerContext, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &sharedServerContext{
		leaseManager:            config.leaseManager,
		logger:                  config.logger,
		controllerUUID:          config.controllerUUID,
		controllerModelUUID:     config.controllerModelUUID,
		controllerConfig:        config.controllerConfig,
		charmhubHTTPClient:      config.charmhubHTTPClient,
		dbGetter:                config.dbGetter,
		dbDeleter:               config.dbDeleter,
		domainServicesGetter:    config.domainServicesGetter,
		controllerConfigService: config.controllerConfigService,
		tracerGetter:            config.tracerGetter,
		objectStoreGetter:       config.objectStoreGetter,
		watcherRegistryGetter:   config.watcherRegistryGetter,
		machineTag:              config.machineTag,
		dataDir:                 config.dataDir,
		logDir:                  config.logDir,
		features:                config.controllerConfig.Features(),
	}, nil
}

func (c *sharedServerContext) updateControllerConfig(ctx context.Context, config controller.Config) {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	c.controllerConfig = config

	features := config.Features()

	removed := c.features.Difference(features)
	added := features.Difference(c.features)
	values := features.SortedValues()

	if removed.Size() != 0 || added.Size() != 0 {
		c.logger.Infof(ctx, "updating features to %v", values)
	}

	c.features = features
}

func (c *sharedServerContext) maxDebugLogDuration() time.Duration {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()

	return c.controllerConfig.MaxDebugLogDuration()
}
