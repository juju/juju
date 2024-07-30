// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/facade"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/state"
)

// SharedHub represents the methods of the pubsub.StructuredHub
// that are used. The context uses an interface to allow mocking
// of the hub.
type SharedHub interface {
	Publish(topic string, data interface{}) (func(), error)
	Subscribe(topic string, handler interface{}) (func(), error)
}

// sharedServerContext contains a number of components that are unchangeable in the API server.
// These components need to be exposed through the facade.ModelContext. Instead of having the methods
// of newAPIHandler and newAPIRoot take ever-increasing numbers of parameters, they will instead
// have a pointer to the sharedServerContext.
//
// All attributes in the context should be goroutine aware themselves, like the state pool, hub, and
// presence, or protected and only accessed through methods on this context object.
type sharedServerContext struct {
	statePool          *state.StatePool
	centralHub         SharedHub
	presence           presence.Recorder
	leaseManager       lease.Manager
	logger             corelogger.Logger
	charmhubHTTPClient facade.HTTPClient

	// sshImporterHTTPClient is the http client used for ssh public key import
	// operations.
	sshImporterHTTPClient facade.HTTPClient

	// dbGetter is used to access databases from the API server. Along with
	// creating a new database for new models and during model migrations.
	dbGetter changestream.WatchableDBGetter

	// dbDeleter is used to delete the database when a model migration fails
	// and the model is being removed.
	dbDeleter database.DBDeleter

	// ServiceFactoryGetter is used to get the service factory for controllers
	// and models.
	serviceFactoryGetter servicefactory.ServiceFactoryGetter

	// TraceGetter is used to get the tracer for the API server.
	tracerGetter trace.TracerGetter

	// ObjectStoreGetter is used to get the object store for storing blobs
	// for the API server.
	objectStoreGetter objectstore.ObjectStoreGetter

	configMutex sync.RWMutex

	// controllerUUID is the unique identifier of the controller.
	controllerUUID   string
	controllerConfig jujucontroller.Config
	features         set.Strings

	// controllerModelID is the ID of the controller model.
	controllerModelID model.UUID

	machineTag names.Tag
	dataDir    string
	logDir     string

	unsubscribe func()
}

type sharedServerConfig struct {
	statePool          *state.StatePool
	centralHub         SharedHub
	presence           presence.Recorder
	leaseManager       lease.Manager
	controllerUUID     string
	controllerModelID  model.UUID
	controllerConfig   jujucontroller.Config
	logger             corelogger.Logger
	charmhubHTTPClient facade.HTTPClient

	// sshImporterHTTPClient is the http client used for ssh public key import
	// operations.
	sshImporterHTTPClient facade.HTTPClient

	dbGetter             changestream.WatchableDBGetter
	dbDeleter            database.DBDeleter
	serviceFactoryGetter servicefactory.ServiceFactoryGetter
	tracerGetter         trace.TracerGetter
	objectStoreGetter    objectstore.ObjectStoreGetter
	machineTag           names.Tag
	dataDir              string
	logDir               string
}

func (c *sharedServerConfig) validate() error {
	if c.statePool == nil {
		return errors.NotValidf("nil statePool")
	}
	if c.centralHub == nil {
		return errors.NotValidf("nil centralHub")
	}
	if c.presence == nil {
		return errors.NotValidf("nil presence")
	}
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
	if c.serviceFactoryGetter == nil {
		return errors.NotValidf("nil serviceFactoryGetter")
	}
	if c.tracerGetter == nil {
		return errors.NotValidf("nil tracerGetter")
	}
	if c.objectStoreGetter == nil {
		return errors.NotValidf("nil objectStoreGetter")
	}
	if c.machineTag == nil {
		return errors.NotValidf("empty machineTag")
	}
	if c.sshImporterHTTPClient == nil {
		return errors.NotValidf("empty sshImporterHTTPClient")
	}
	return nil
}

func newSharedServerContext(config sharedServerConfig) (*sharedServerContext, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	ctx := &sharedServerContext{
		statePool:             config.statePool,
		centralHub:            config.centralHub,
		presence:              config.presence,
		leaseManager:          config.leaseManager,
		logger:                config.logger,
		controllerUUID:        config.controllerUUID,
		controllerModelID:     config.controllerModelID,
		controllerConfig:      config.controllerConfig,
		charmhubHTTPClient:    config.charmhubHTTPClient,
		sshImporterHTTPClient: config.sshImporterHTTPClient,
		dbGetter:              config.dbGetter,
		dbDeleter:             config.dbDeleter,
		serviceFactoryGetter:  config.serviceFactoryGetter,
		tracerGetter:          config.tracerGetter,
		objectStoreGetter:     config.objectStoreGetter,
		machineTag:            config.machineTag,
		dataDir:               config.dataDir,
		logDir:                config.logDir,
	}
	ctx.features = config.controllerConfig.Features()
	// We are able to get the current controller config before subscribing to changes
	// because the changes are only ever published in response to an API call, and
	// this function is called in the newServer call to create the API server,
	// and we know that we can't make any API calls until the server has started.
	unsubscribe, err := ctx.centralHub.Subscribe(controller.ConfigChanged, ctx.onConfigChanged)
	if err != nil {
		ctx.logger.Criticalf("programming error in subscribe function: %v", err)
		return nil, errors.Trace(err)
	}
	ctx.unsubscribe = unsubscribe
	return ctx, nil
}

func (c *sharedServerContext) Close() {
	c.unsubscribe()
}

func (c *sharedServerContext) onConfigChanged(topic string, data controller.ConfigChangedMessage, err error) {
	if err != nil {
		c.logger.Criticalf("programming error in %s message data: %v", topic, err)
		return
	}

	features := data.Config.Features()

	c.configMutex.Lock()
	c.controllerConfig = data.Config
	removed := c.features.Difference(features)
	added := features.Difference(c.features)
	c.features = features
	values := features.SortedValues()
	c.configMutex.Unlock()

	if removed.Size() != 0 || added.Size() != 0 {
		c.logger.Infof("updating features to %v", values)
	}
}

func (c *sharedServerContext) maxDebugLogDuration() time.Duration {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	return c.controllerConfig.MaxDebugLogDuration()
}
