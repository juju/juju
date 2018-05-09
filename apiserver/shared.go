// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils/set"

	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/pubsub/controller"
	"github.com/juju/juju/state"
)

// sharedServerContext contains a number of components that are unchangeable in the API server.
// These components need to be exposed through the facade.Context. Instead of having the methods
// of newAPIHandler and newAPIRoot take ever increasing numbers of parameters, they will instead
// have a pointer to the sharedServerContext.
//
// All attributes in the context should be goroutine aware themselves, like the state pool, hub, and
// presence, or protected and only accessed through methods on this context object.
type sharedServerContext struct {
	statePool  *state.StatePool
	centralHub *pubsub.StructuredHub
	presence   presence.Recorder
	logger     loggo.Logger

	featuresMutex sync.RWMutex
	features      set.Strings

	unsubscribe func()
}

type sharedServerConfig struct {
	statePool  *state.StatePool
	centralHub *pubsub.StructuredHub
	presence   presence.Recorder
	logger     loggo.Logger
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
	return nil
}

func newSharedServerContex(config sharedServerConfig) (*sharedServerContext, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	ctx := &sharedServerContext{
		statePool:  config.statePool,
		centralHub: config.centralHub,
		presence:   config.presence,
		logger:     config.logger,
	}
	controllerConfig, err := ctx.statePool.SystemState().ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	ctx.features = controllerConfig.Features()
	// We are able to get the current controller config before subscribing to changes
	// because the changes are only ever published in response to an API call, and
	// this function is called in the newServer call to create the API server,
	// and we know that we can't make any API calls until the server has started.
	ctx.unsubscribe, err = ctx.centralHub.Subscribe(controller.ConfigChanged, ctx.onConfigChanged)
	if err != nil {
		ctx.logger.Criticalf("programming error in subscribe function: %v", err)
		return nil, errors.Trace(err)
	}
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

	c.featuresMutex.Lock()
	removed := c.features.Difference(features)
	added := features.Difference(c.features)
	c.features = features
	values := features.SortedValues()
	c.featuresMutex.Unlock()

	if removed.Size() != 0 || added.Size() != 0 {
		c.logger.Infof("updating features to %v", values)
	}
}

func (c *sharedServerContext) featureEnabled(flag string) bool {
	c.featuresMutex.RLock()
	defer c.featuresMutex.RUnlock()
	return c.features.Contains(flag)
}
