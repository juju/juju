// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"gopkg.in/tomb.v2"
)

// We use a package level logger here because the cache is only
// ever in machine agents, so will never need to be in an alternative
// logging context.

// ControllerConfig is a simple config value struct for the controller.
type ControllerConfig struct {
	// Changes from the event source come over this channel.
	// The changes channel must be non-nil.
	Changes <-chan interface{}

	// Notify is a callback function used primarily for testing, and is
	// called by the controller main processing loop after processing a change.
	// The change processed is passed in as the arg to notify.
	Notify func(interface{})
}

// Validate ensures the controller has the right values to be created.
func (c *ControllerConfig) Validate() error {
	if c.Changes == nil {
		return errors.NotValidf("nil Changes")
	}
	return nil
}

// Controller is the primary cached object.
type Controller struct {
	config  ControllerConfig
	tomb    tomb.Tomb
	mu      sync.Mutex
	models  map[string]*Model
	hub     *pubsub.SimpleHub
	metrics *ControllerGauges
}

// NewController creates a new cached controller intance.
// The changes channel is what is used to supply the cache with the changes
// in order for the cache to be kept up to date.
func NewController(config ControllerConfig) (*Controller, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	c := &Controller{
		config: config,
		models: make(map[string]*Model),
		hub: pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
			// TODO: (thumper) add a get child method to loggers.
			Logger: loggo.GetLogger("juju.core.cache.hub"),
		}),
		metrics: createControllerGauges(),
	}
	c.tomb.Go(c.loop)
	return c, nil
}

func (c *Controller) loop() error {
	for {
		select {
		case <-c.tomb.Dying():
			return nil
		case change := <-c.config.Changes:
			switch ch := change.(type) {
			case ModelChange:
				c.updateModel(ch)
			case RemoveModel:
				c.removeModel(ch)
			case ApplicationChange:
				c.updateApplication(ch)
			case RemoveApplication:
				c.removeApplication(ch)
			case UnitChange:
				c.updateUnit(ch)
			case RemoveUnit:
				c.removeUnit(ch)
			}
			if c.config.Notify != nil {
				c.config.Notify(change)
			}
		}
	}
}

// Report returns information that is used in the dependency engine report.
func (c *Controller) Report() map[string]interface{} {
	result := make(map[string]interface{})

	c.mu.Lock()
	for uuid, model := range c.models {
		result[uuid] = model.Report()
	}
	c.mu.Unlock()

	return result
}

// ModelUUIDs returns the UUIDs of the models in the cache.
func (c *Controller) ModelUUIDs() []string {
	c.mu.Lock()

	result := make([]string, 0, len(c.models))
	for uuid := range c.models {
		result = append(result, uuid)
	}

	c.mu.Unlock()
	return result
}

// Kill is part of the worker.Worker interface.
func (c *Controller) Kill() {
	c.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (c *Controller) Wait() error {
	return c.tomb.Wait()
}

// Model returns the model for the specified UUID.
// If the model isn't found, a NotFoundError is returned.
func (c *Controller) Model(uuid string) (*Model, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	model, found := c.models[uuid]
	if !found {
		return nil, errors.NotFoundf("model %q", uuid)
	}
	return model, nil
}

// updateModel will add or update the model details as
// described in the ModelChange.
func (c *Controller) updateModel(ch ModelChange) {
	c.mu.Lock()
	c.ensureModel(ch.ModelUUID).setDetails(ch)
	c.mu.Unlock()
}

// removeModel removes the model from the cache.
func (c *Controller) removeModel(ch RemoveModel) {
	c.mu.Lock()
	delete(c.models, ch.ModelUUID)
	c.mu.Unlock()
}

// updateApplication adds or updates the application in the specified model.
func (c *Controller) updateApplication(ch ApplicationChange) {
	c.mu.Lock()
	c.ensureModel(ch.ModelUUID).updateApplication(ch)
	c.mu.Unlock()
}

// removeApplication removes the application for the cached model.
// If the cache does not have the model loaded for the application yet,
// then it will not have the application cached.
func (c *Controller) removeApplication(ch RemoveApplication) {
	c.mu.Lock()
	if model, ok := c.models[ch.ModelUUID]; ok {
		model.removeApplication(ch)
	}
	c.mu.Unlock()
}

// updateApplication adds or updates the application in the specified model.
func (c *Controller) updateUnit(ch UnitChange) {
	c.mu.Lock()
	c.ensureModel(ch.ModelUUID).updateUnit(ch)
	c.mu.Unlock()
}

// removeUnit removes the unit from the cached model.
// If the cache does not have the model loaded for the unit yet,
// then it will not have the unit cached.
func (c *Controller) removeUnit(ch RemoveUnit) {
	c.mu.Lock()
	if model, ok := c.models[ch.ModelUUID]; ok {
		model.removeUnit(ch)
	}
	c.mu.Unlock()
}

// ensureModel retrieves the cached model for the input UUID,
// or adds it if not found.
// It is likely that we will receive a change update for the model before we
// get an update for one of its entities, but the cache needs to be resilient
// enough to make sure that we can handle when this is not the case.
func (c *Controller) ensureModel(modelUUID string) *Model {
	model, found := c.models[modelUUID]
	if !found {
		model = newModel(c.metrics, c.hub)
		c.models[modelUUID] = model
	}
	return model
}
