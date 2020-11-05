// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"gopkg.in/tomb.v2"
)

// We use a package level logger here because the cache is only
// ever in machine agents, so will never need to be in an alternative
// logging context.

// Controller pubsub topics
const (
	// A model has been updated in the controller.
	modelUpdatedTopic = "model-updated"
	// A model has been removed from the controller.
	modelRemovedTopic = "model-removed"
	// A model summary has changed
	modelSummaryUpdatedTopic = "model-summary-changed"

	// modelAppearingTimeout is how long the controller will wait for a model to
	// exist before it either times out or returns a not found.
	modelAppearingTimeout = 5 * time.Second
)

var (
	// Idle* vars below are used for testing. Last changed by tlm on 05/11/20
	// Changed IdleTime to a larger value to allow high churn tests to catch up

	// IdleFunc allows tests to be able to get callbacks when the controller
	// hasn't been given any changes for a specified time.
	IdleFunc func()

	// IdleTime relates to how long the controller needs to wait with no changes
	// to be considered idle.
	IdleTime = 200 * time.Millisecond
)

// Clock defines the clockish methods used by the controller.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

// ControllerConfig is a simple config value struct for the controller.
type ControllerConfig struct {
	// Changes from the event source come over this channel.
	// The changes channel must be non-nil.
	Changes chan interface{}

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
	// manager is used to work with cache residents
	// from a type-agnostic viewpoint.
	manager *residentManager

	changes  <-chan interface{}
	notify   func(interface{})
	idleFunc func()
	hub      *pubsub.SimpleHub
	models   map[string]*Model

	tomb    tomb.Tomb
	metrics *ControllerGauges

	// config is the controller config.
	configMu sync.Mutex
	config   map[string]interface{}

	// While a controller is initializing it does not update any model
	// summaries - we want to avoid publishing events related to cache priming.
	// The initialization status is handled with the Mark and Sweep methods.
	// Calling Mark sets the controller as initializing, and Sweep completes
	// the initialization.
	// This status is shared with the cached models by passing the controller's
	// isInitialising method.
	initializing bool

	// modelsMu protects access to the controller's collection of models.
	modelsMu sync.Mutex

	// initMu protects access to the controller's initialization status.
	// We do this separately from models, as we may need to synchronise access
	// to both aspects at once without deadlocking.
	initMu sync.Mutex
}

// NewController creates a new cached controller instance.
// The changes channel is what is used to supply the cache with the changes
// in order for the cache to be kept up to date.
func NewController(config ControllerConfig) (*Controller, error) {
	c, err := newController(config, newResidentManager(config.Changes))
	return c, errors.Trace(err)
}

// newController is the internal constructor that allows supply of a manager.
func newController(config ControllerConfig, manager *residentManager) (*Controller, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	c := &Controller{
		manager:  manager,
		changes:  config.Changes,
		notify:   config.Notify,
		idleFunc: IdleFunc,
		hub:      newPubSubHub(),
		models:   make(map[string]*Model),
		metrics:  createControllerGauges(),
	}

	manager.dying = c.tomb.Dying()
	c.tomb.Go(c.loop)
	return c, nil
}

func (c *Controller) loop() error {
	var idle <-chan time.Time
	if c.idleFunc != nil {
		logger.Tracef("controller %p set idle timeout to %s", c, IdleTime)
		idle = time.After(IdleTime)
	}
	for {
		select {
		case <-c.tomb.Dying():
			return nil
		case <-idle:
			logger.Tracef("controller %p is idle", c)
			c.idleFunc()
			idle = time.After(IdleTime)
		case change := <-c.changes:
			var err error

			switch ch := change.(type) {
			case ControllerConfigChange:
				c.configMu.Lock()
				c.config = ch.Config
				c.configMu.Unlock()
			case ModelChange:
				c.updateModel(ch)
			case RemoveModel:
				err = c.removeModel(ch)
			case ApplicationChange:
				c.updateApplication(ch)
			case RemoveApplication:
				err = c.removeApplication(ch)
			case CharmChange:
				c.updateCharm(ch)
			case RemoveCharm:
				err = c.removeCharm(ch)
			case MachineChange:
				c.updateMachine(ch)
			case RemoveMachine:
				err = c.removeMachine(ch)
			case UnitChange:
				c.updateUnit(ch)
			case RemoveUnit:
				err = c.removeUnit(ch)
			case RelationChange:
				c.updateRelation(ch)
			case RemoveRelation:
				err = c.removeRelation(ch)
			case BranchChange:
				c.updateBranch(ch)
			case RemoveBranch:
				err = c.removeBranch(ch)
			}
			if c.notify != nil {
				c.notify(change)
			}

			if err != nil {
				logger.Errorf("processing cache change: %s", err.Error())
			}

			if c.idleFunc != nil {
				idle = time.After(IdleTime)
			}
		}
	}
}

// Mark updates all cached entities to indicate they are stale.
func (c *Controller) Mark() {
	c.manager.mark()
	c.setInitializing(true)
}

// Sweep evicts any stale entities from the cache,
// cleaning up resources that they are responsible for.
func (c *Controller) Sweep() {
	select {
	case <-c.manager.sweep():
	case <-c.tomb.Dying():
	}

	// If we are not currently initialising, then this call to `Sweep` was not
	// the first after a `Mark`. This means that the cache is primed and
	// the last summaries published reflect the correct state of the world.
	// No need to proceed from here.
	publishSummaries := c.isInitializing()
	c.setInitializing(false)
	if !publishSummaries {
		return
	}

	// When this call to `Sweep` is the first after a `Mark`, we will have been
	// in initialization mode, and updates to cached models will not have
	// caused summaries to be published.
	// Now that the we are primed, publish all the summary data.
	c.modelsMu.Lock()
	for _, model := range c.models {
		model.mu.Lock()
		model.updateSummary()
		model.mu.Unlock()
	}
	c.modelsMu.Unlock()
}

// Report returns information that is used in the dependency engine report.
func (c *Controller) Report() map[string]interface{} {
	result := make(map[string]interface{})

	c.modelsMu.Lock()
	for uuid, model := range c.models {
		result[uuid] = model.Report()
	}
	c.modelsMu.Unlock()

	return result
}

// ModelUUIDs returns the UUIDs of the models in the cache.
func (c *Controller) ModelUUIDs() []string {
	c.modelsMu.Lock()

	result := make([]string, 0, len(c.models))
	for uuid := range c.models {
		result = append(result, uuid)
	}

	c.modelsMu.Unlock()
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

// Name returns the controller-name from the controller config.
func (c *Controller) Name() string {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	value := c.config["controller-name"]
	if name, ok := value.(string); ok {
		return name
	}
	return ""
}

// Model returns the model for the specified UUID.
// If the model isn't found, a NotFoundError is returned.
func (c *Controller) Model(uuid string) (*Model, error) {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()

	model, found := c.models[uuid]
	if !found {
		return nil, errors.NotFoundf("model %q", uuid)
	}
	return model, nil
}

// WaitForModel waits for a time for the specified model to appear in the cache.
func (c *Controller) WaitForModel(uuid string, clock Clock) (*Model, error) {
	watcher := c.modelWatcher(uuid)
	defer watcher.Kill()
	select {
	case <-clock.After(modelAppearingTimeout):
		return nil, errors.Timeoutf("model %q did not appear in cache", uuid)
	case model := <-watcher.Changes():
		return model, nil
	}
}

// modelWatcher creates a watcher that will pass the Model
// down the changes channel when it becomes available. It may
// be immediately available.
func (c *Controller) modelWatcher(uuid string) ModelWatcher {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()

	model, _ := c.models[uuid]
	return newModelWatcher(uuid, c.hub, model)
}

// updateModel will add or update the model details as
// described in the ModelChange.
func (c *Controller) updateModel(ch ModelChange) {
	model := c.ensureModel(ch.ModelUUID)
	model.setDetails(ch)
	c.hub.Publish(modelUpdatedTopic, model)
}

// removeModel removes the model from the cache.
func (c *Controller) removeModel(ch RemoveModel) error {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()

	mod, ok := c.models[ch.ModelUUID]
	if ok {
		if err := mod.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(c.models, ch.ModelUUID)
		c.hub.Publish(modelRemovedTopic, ch.ModelUUID)
	}
	return nil
}

// updateApplication adds or updates the application in the specified model.
func (c *Controller) updateApplication(ch ApplicationChange) {
	c.ensureModel(ch.ModelUUID).updateApplication(ch, c.manager)
}

// removeApplication removes the application for the cached model.
func (c *Controller) removeApplication(ch RemoveApplication) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeApplication(ch) }))
}

func (c *Controller) updateCharm(ch CharmChange) {
	c.ensureModel(ch.ModelUUID).updateCharm(ch, c.manager)
}

func (c *Controller) removeCharm(ch RemoveCharm) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeCharm(ch) }))
}

// updateUnit adds or updates the unit in the specified model.
func (c *Controller) updateUnit(ch UnitChange) {
	c.ensureModel(ch.ModelUUID).updateUnit(ch, c.manager)
}

// removeUnit removes the unit from the cached model.
func (c *Controller) removeUnit(ch RemoveUnit) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeUnit(ch) }))
}

// updateRelation adds or updates the relation in the specified model.
func (c *Controller) updateRelation(ch RelationChange) {
	c.ensureModel(ch.ModelUUID).updateRelation(ch, c.manager)
}

// removeRelation removes the relation from the cached model.
func (c *Controller) removeRelation(ch RemoveRelation) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeRelation(ch) }))
}

// updateMachine adds or updates the machine in the specified model.
func (c *Controller) updateMachine(ch MachineChange) {
	c.ensureModel(ch.ModelUUID).updateMachine(ch, c.manager)
}

// removeMachine removes the machine from the cached model.
func (c *Controller) removeMachine(ch RemoveMachine) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeMachine(ch) }))
}

// updateBranch adds or updates the branch in the specified model.
func (c *Controller) updateBranch(ch BranchChange) {
	c.ensureModel(ch.ModelUUID).updateBranch(ch, c.manager)
}

// removeBranch removes the branch from the cached model.
func (c *Controller) removeBranch(ch RemoveBranch) error {
	return errors.Trace(c.removeResident(ch.ModelUUID, func(m *Model) error { return m.removeBranch(ch) }))
}

// removeResident uses the input removal function to remove a cache resident,
// including cleaning up resources it was responsible for creating.
// If the cache does not have the model loaded for the resident yet,
// then it will not have the entity cached, and a no-op results.
func (c *Controller) removeResident(modelUUID string, removeFrom func(m *Model) error) error {
	c.modelsMu.Lock()

	var err error
	if model, ok := c.models[modelUUID]; ok {
		err = removeFrom(model)
	}

	c.modelsMu.Unlock()
	return errors.Trace(err)
}

// ensureModel retrieves the cached model for the input UUID,
// or adds it if not found.
// It is likely that we will receive a change update for the model before we
// get an update for one of its entities, but the cache needs to be resilient
// enough to make sure that we can handle when this is not the case.
// No model returned by this method is ever considered to be stale.
func (c *Controller) ensureModel(modelUUID string) *Model {
	c.modelsMu.Lock()

	model, found := c.models[modelUUID]
	if !found {
		model = newModel(modelConfig{
			initializing: c.isInitializing,
			metrics:      c.metrics,
			hub:          newPubSubHub(),
			chub:         c.hub,
			res:          c.manager.new(),
		})
		c.models[modelUUID] = model
	} else {
		model.setStale(false)
	}

	c.modelsMu.Unlock()
	return model
}

func (c *Controller) isInitializing() bool {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	return c.initializing
}

func (c *Controller) setInitializing(init bool) {
	c.initMu.Lock()
	c.initializing = init
	c.initMu.Unlock()
}

// WatchModelsAsUser returns a watcher that will signal whenever there are
// changes in the summary for that model. Only models the user can see are
// included in the results.
func (c *Controller) WatchModelsAsUser(username string) ModelSummaryWatcher {
	return newModelSummaryWatcher(c, username)
}

// WatchAllModels returns a watcher that will signal whenever there are
// changes in the summary for that model.
func (c *Controller) WatchAllModels() ModelSummaryWatcher {
	return newModelSummaryWatcher(c, "")
}

func newPubSubHub() *pubsub.SimpleHub {
	return pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		// TODO: (thumper) add a get child method to loggers.
		Logger: loggo.GetLogger("juju.core.cache.hub"),
	})
}
