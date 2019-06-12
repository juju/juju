// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/modelcache"
)

type WorkerSuite struct {
	statetesting.StateSuite
	logger loggo.Logger
	config modelcache.Config
	notify func(interface{})
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.notify = nil
	s.logger = loggo.GetLogger("test")
	s.logger.SetLogLevel(loggo.TRACE)

	s.config = modelcache.Config{
		Logger: s.logger,
		WatcherFactory: func() modelcache.BackingWatcher {
			return s.StatePool.SystemState().WatchAllModels(s.StatePool)
		},
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	}
}

func (s *WorkerSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing logger not valid")
}

func (s *WorkerSuite) TestConfigMissingWatcherFactory(c *gc.C) {
	s.config.WatcherFactory = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing watcher factory not valid")
}

func (s *WorkerSuite) TestConfigMissingRegisterer(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing prometheus registerer not valid")
}

func (s *WorkerSuite) TestConfigMissingCleanup(c *gc.C) {
	s.config.Cleanup = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing cleanup func not valid")
}

func (s *WorkerSuite) getController(c *gc.C, w worker.Worker) *cache.Controller {
	var controller *cache.Controller
	err := modelcache.ExtractCacheController(w, &controller)
	c.Assert(err, jc.ErrorIsNil)
	return controller
}

func (s *WorkerSuite) TestExtractCacheController(c *gc.C) {
	var controller *cache.Controller
	var empty worker.Worker
	err := modelcache.ExtractCacheController(empty, &controller)
	c.Assert(err, gc.ErrorMatches, `in should be a \*modelcache.cacheWorker; got <nil>`)
}

func (s *WorkerSuite) start(c *gc.C) worker.Worker {
	config := s.config
	config.Notify = s.notify
	w, err := modelcache.NewWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, w)
	})
	return w
}

func (s *WorkerSuite) checkModel(c *gc.C, obtained interface{}, model *state.Model) {
	change, ok := obtained.(cache.ModelChange)
	c.Assert(ok, jc.IsTrue)

	c.Check(change.ModelUUID, gc.Equals, model.UUID())
	c.Check(change.Name, gc.Equals, model.Name())
	c.Check(change.Life, gc.Equals, life.Value(model.Life().String()))
	c.Check(change.Owner, gc.Equals, model.Owner().Name())
	cfg, err := model.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(change.Config, jc.DeepEquals, cfg.AllAttrs())
	status, err := model.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(change.Status, jc.DeepEquals, status)
}

func (s *WorkerSuite) TestInitialModel(c *gc.C) {
	changes := s.captureEvents(c, modelEvents)
	s.start(c)

	obtained := s.nextChange(c, changes)
	expected, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.checkModel(c, obtained, expected)
}

func (s *WorkerSuite) TestModelConfigChange(c *gc.C) {
	changes := s.captureEvents(c, modelEvents)
	w := s.start(c)
	// discard initial event
	s.nextChange(c, changes)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("\nupdating status\n\n")

	// Add a different logging config value.
	expected := "juju=INFO;missing=DEBUG;unit=DEBUG"
	err = model.UpdateModelConfig(map[string]interface{}{
		"logging-config": expected,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()

	// Wait for the change.
	s.nextChange(c, changes)

	controller := s.getController(c, w)
	cachedModel, err := controller.Model(s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cachedModel.Config()["logging-config"], gc.Equals, expected)
}

func (s *WorkerSuite) TestNewModel(c *gc.C) {
	changes := s.captureEvents(c, modelEvents)
	w := s.start(c)
	// grab and discard the event for the initial model
	s.nextChange(c, changes)

	newState := s.Factory.MakeModel(c, nil)
	s.State.StartSync()
	defer func() { _ = newState.Close() }()

	obtained := s.nextChange(c, changes)
	expected, err := newState.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.checkModel(c, obtained, expected)

	controller := s.getController(c, w)
	c.Assert(controller.ModelUUIDs(), gc.HasLen, 2)
}

func (s *WorkerSuite) TestRemovedModel(c *gc.C) {
	changes := s.captureEvents(c, modelEvents)
	w := s.start(c)

	// grab and discard the event for the initial model
	s.nextChange(c, changes)

	st := s.Factory.MakeModel(c, nil)
	s.State.StartSync()
	defer func() { _ = st.Close() }()

	// grab and discard the event for the new model
	s.nextChange(c, changes)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()

	// grab and discard the event for the new model
	obtained := s.nextChange(c, changes)
	modelChange, ok := obtained.(cache.ModelChange)
	c.Assert(ok, jc.IsTrue)
	c.Assert(modelChange.Life, gc.Equals, life.Value("dying"))

	err = st.ProcessDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()

	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()

	obtained = s.nextChange(c, changes)

	change, ok := obtained.(cache.RemoveModel)
	c.Assert(ok, jc.IsTrue)
	c.Check(change.ModelUUID, gc.Equals, model.UUID())

	// Controller just has the system state again.
	controller := s.getController(c, w)
	c.Assert(controller.ModelUUIDs(), jc.SameContents, []string{s.State.ModelUUID()})
}

func (s *WorkerSuite) TestAddApplication(c *gc.C) {
	changes := s.captureEvents(c, applicationEvents)
	w := s.start(c)

	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{})
	s.State.StartSync()

	change := s.nextChange(c, changes)
	obtained, ok := change.(cache.ApplicationChange)
	c.Assert(ok, jc.IsTrue)
	c.Check(obtained.Name, gc.Equals, app.Name())

	controller := s.getController(c, w)
	modUUIDs := controller.ModelUUIDs()
	c.Check(modUUIDs, gc.HasLen, 1)

	mod, err := controller.Model(modUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	cachedApp, err := mod.Application(app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cachedApp, gc.NotNil)
}

func (s *WorkerSuite) TestRemoveApplication(c *gc.C) {
	changes := s.captureEvents(c, applicationEvents)
	w := s.start(c)

	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{})
	s.State.StartSync()
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)
	modUUID := controller.ModelUUIDs()[0]

	c.Assert(app.Destroy(), jc.ErrorIsNil)
	s.State.StartSync()

	// We will either get our application event,
	// or time-out after processing all the changes.
	for {
		change := s.nextChange(c, changes)
		if _, ok := change.(cache.RemoveApplication); ok {
			mod, err := controller.Model(modUUID)
			c.Assert(err, jc.ErrorIsNil)

			_, err = mod.Application(app.Name())
			c.Check(errors.IsNotFound(err), jc.IsTrue)
			return
		}
	}
}

func (s *WorkerSuite) TestAddMachine(c *gc.C) {
	changes := s.captureEvents(c, machineEvents)
	w := s.start(c)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{})
	s.State.StartSync()

	change := s.nextChange(c, changes)
	obtained, ok := change.(cache.MachineChange)
	c.Assert(ok, jc.IsTrue)
	c.Check(obtained.Id, gc.Equals, machine.Id())

	controller := s.getController(c, w)
	modUUIDs := controller.ModelUUIDs()
	c.Check(modUUIDs, gc.HasLen, 1)

	mod, err := controller.Model(modUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	cachedMachine, err := mod.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cachedMachine, gc.NotNil)
}

func (s *WorkerSuite) TestRemoveMachine(c *gc.C) {
	changes := s.captureEvents(c, machineEvents)
	w := s.start(c)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{})
	s.State.StartSync()
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)
	modUUID := controller.ModelUUIDs()[0]

	// Move machine to dying.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	// Move machine to dead.
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	// Remove will delete the machine from the database.
	c.Assert(machine.Remove(), jc.ErrorIsNil)
	s.State.StartSync()

	// We will either get our machine event,
	// or time-out after processing all the changes.
	for {
		change := s.nextChange(c, changes)
		if _, ok := change.(cache.RemoveMachine); ok {
			mod, err := controller.Model(modUUID)
			c.Assert(err, jc.ErrorIsNil)

			_, err = mod.Machine(machine.Id())
			c.Check(errors.IsNotFound(err), jc.IsTrue)
			return
		}
	}
}

func (s *WorkerSuite) TestAddCharm(c *gc.C) {
	changes := s.captureEvents(c, charmEvents)
	w := s.start(c)

	charm := s.Factory.MakeCharm(c, &factory.CharmParams{})
	s.State.StartSync()

	change := s.nextChange(c, changes)
	obtained, ok := change.(cache.CharmChange)
	c.Assert(ok, jc.IsTrue)
	c.Check(obtained.CharmURL, gc.Equals, charm.URL().String())

	controller := s.getController(c, w)
	modUUIDs := controller.ModelUUIDs()
	c.Check(modUUIDs, gc.HasLen, 1)

	mod, err := controller.Model(modUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	cachedCharm, err := mod.Charm(charm.URL().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cachedCharm, gc.NotNil)
}

func (s *WorkerSuite) TestRemoveCharm(c *gc.C) {
	changes := s.captureEvents(c, charmEvents)
	w := s.start(c)

	charm := s.Factory.MakeCharm(c, &factory.CharmParams{})
	s.State.StartSync()
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)
	modUUID := controller.ModelUUIDs()[0]

	// Move charm to dying.
	c.Assert(charm.Destroy(), jc.ErrorIsNil)
	// Remove will delete the charm from the database.
	c.Assert(charm.Remove(), jc.ErrorIsNil)
	s.State.StartSync()

	// We will either get our charm event,
	// or time-out after processing all the changes.
	for {
		change := s.nextChange(c, changes)
		if _, ok := change.(cache.RemoveCharm); ok {
			mod, err := controller.Model(modUUID)
			c.Assert(err, jc.ErrorIsNil)

			_, err = mod.Charm(charm.URL().String())
			c.Check(errors.IsNotFound(err), jc.IsTrue)
			return
		}
	}
}

func (s *WorkerSuite) TestAddUnit(c *gc.C) {
	changes := s.captureEvents(c, unitEvents)
	w := s.start(c)

	unit := s.Factory.MakeUnit(c, &factory.UnitParams{})
	s.State.StartSync()

	change := s.nextChange(c, changes)
	obtained, ok := change.(cache.UnitChange)
	c.Assert(ok, jc.IsTrue)
	c.Check(obtained.Name, gc.Equals, unit.Name())
	c.Check(obtained.Application, gc.Equals, unit.ApplicationName())

	controller := s.getController(c, w)
	modUUIDs := controller.ModelUUIDs()
	c.Check(modUUIDs, gc.HasLen, 1)

	mod, err := controller.Model(modUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	cachedApp, err := mod.Application(unit.ApplicationName())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cachedApp, gc.NotNil)
}

func (s *WorkerSuite) TestRemoveUnit(c *gc.C) {
	changes := s.captureEvents(c, unitEvents)
	w := s.start(c)

	unit := s.Factory.MakeUnit(c, &factory.UnitParams{})
	s.State.StartSync()
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)
	modUUID := controller.ModelUUIDs()[0]

	c.Assert(unit.Destroy(), jc.ErrorIsNil)
	s.State.StartSync()

	// We will either get our unit event,
	// or time-out after processing all the changes.
	for {
		change := s.nextChange(c, changes)
		if _, ok := change.(cache.RemoveUnit); ok {
			mod, err := controller.Model(modUUID)
			c.Assert(err, jc.ErrorIsNil)

			_, err = mod.Unit(unit.Name())
			c.Check(errors.IsNotFound(err), jc.IsTrue)
			return
		}
	}
}

func (s *WorkerSuite) TestAddBranch(c *gc.C) {
	changes := s.captureEvents(c, branchEvents)
	w := s.start(c)

	branchName := "test-branch"
	c.Assert(s.State.AddBranch(branchName, "test-user"), jc.ErrorIsNil)
	s.State.StartSync()

	change := s.nextChange(c, changes)
	obtained, ok := change.(cache.BranchChange)
	c.Assert(ok, jc.IsTrue)
	c.Check(obtained.Name, gc.Equals, "test-branch")

	controller := s.getController(c, w)
	modUUIDs := controller.ModelUUIDs()
	c.Check(modUUIDs, gc.HasLen, 1)

	mod, err := controller.Model(modUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	_, err = mod.Branch(branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestRemoveBranch(c *gc.C) {
	changes := s.captureEvents(c, branchEvents)
	w := s.start(c)

	branchName := "test-branch"
	c.Assert(s.State.AddBranch(branchName, "test-user"), jc.ErrorIsNil)
	s.State.StartSync()
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)
	modUUID := controller.ModelUUIDs()[0]

	branch, err := s.State.Branch(branchName)
	c.Assert(err, jc.ErrorIsNil)

	// Generation docs are not deleted from the DB in any current workflow.
	// Committing the branch so that it is no longer active should cause
	// a removal message to be emitted.
	_, err = branch.Commit("test-user")
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()

	// We will either get our branch event,
	// or time-out after processing all the changes.
	for {
		change := s.nextChange(c, changes)
		if _, ok := change.(cache.RemoveBranch); ok {
			mod, err := controller.Model(modUUID)
			c.Assert(err, jc.ErrorIsNil)

			_, err = mod.Branch(branchName)
			c.Check(errors.IsNotFound(err), jc.IsTrue)
			return
		}
	}
}

func (s *WorkerSuite) TestWatcherErrorCacheMarkSweep(c *gc.C) {
	// Some state to close over.
	fakeModelSent := false
	errorSent := false

	s.config.WatcherFactory = func() modelcache.BackingWatcher {
		return testingMultiwatcher{
			Multiwatcher: s.StatePool.SystemState().WatchAllModels(s.StatePool),
			manipulate: func(deltas []multiwatcher.Delta) ([]multiwatcher.Delta, error) {
				if !fakeModelSent || !errorSent {
					for _, delta := range deltas {
						// The first time we see a model, add an extra model delta.
						// This will be cached even though it does not exist in state.
						if delta.Entity.EntityId().Kind == "model" && !fakeModelSent {
							fakeModelSent = true
							return append(deltas, multiwatcher.Delta{
								Entity: &multiwatcher.ModelInfo{
									ModelUUID: "fake-ass-model-uuid",
									Name:      "evict-this-cat",
								},
							}), nil
						}

						// The first time we see an application, throw an error.
						// This will restart the watcher and cause a cache refresh.
						// We expect after this that the application will reside in
						// the cache and our fake model will be removed.
						if delta.Entity.EntityId().Kind == "application" && !errorSent {
							errorSent = true
							return nil, errors.New("boom")
						}
					}
				}
				return deltas, nil
			},
		}
	}

	changes := s.captureEvents(c, modelEvents, applicationEvents)
	w := s.start(c)
	s.State.StartSync()

	// Initial deltas will include the real model and our fake one.
	_ = s.nextChange(c, changes)
	_ = s.nextChange(c, changes)

	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{})
	s.State.StartSync()

	// Watcher will restart and cache will refresh before we see this.
	// These will be the real model and the application,
	// as well as the fake model deletion.
	_ = s.nextChange(c, changes)
	_ = s.nextChange(c, changes)
	_ = s.nextChange(c, changes)

	controller := s.getController(c, w)

	// Only the real model is there.
	models := controller.ModelUUIDs()
	c.Assert(models, gc.HasLen, 1)
	c.Check(models[0], gc.Not(gc.Equals), "fake-ass-model-uuid")

	mod, err := controller.Model(models[0])
	c.Assert(err, jc.ErrorIsNil)

	// The application wound up in the cache,
	// even though we threw an error when we first saw it.
	cachedApp, err := mod.Application(app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cachedApp, gc.NotNil)
}

func (s *WorkerSuite) TestWatcherErrorStoppedKillsWorker(c *gc.C) {
	mw := s.StatePool.SystemState().WatchAllModels(s.StatePool)
	s.config.WatcherFactory = func() modelcache.BackingWatcher { return mw }

	w := s.start(c)

	// Stop the backing multiwatcher.
	c.Assert(mw.Stop(), jc.ErrorIsNil)

	// Check that the worker is killed.
	err := workertest.CheckKilled(c, w)
	c.Assert(err, jc.Satisfies, state.IsErrStopped)
}

func (s *WorkerSuite) captureEvents(c *gc.C, matchers ...func(interface{}) bool) <-chan interface{} {
	events := make(chan interface{})
	s.notify = func(change interface{}) {
		send := false
		for _, m := range matchers {
			if m(change) {
				send = true
				break
			}
		}

		if send {
			c.Logf("sending %#v", change)
			select {
			case events <- change:
			case <-time.After(testing.LongWait):
				c.Fatalf("change not processed by test")
			}
		}
	}
	return events
}

func (s *WorkerSuite) nextChange(c *gc.C, changes <-chan interface{}) interface{} {
	var obtained interface{}
	select {
	case obtained = <-changes:
	case <-time.After(testing.LongWait):
		c.Fatalf("no change")
	}
	return obtained
}

var modelEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.ModelChange:
		return true
	case cache.RemoveModel:
		return true
	}
	return false
}

var applicationEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.ApplicationChange:
		return true
	case cache.RemoveApplication:
		return true
	}
	return false
}

var machineEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.MachineChange:
		return true
	case cache.RemoveMachine:
		return true
	}
	return false
}

var charmEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.CharmChange:
		return true
	case cache.RemoveCharm:
		return true
	}
	return false
}

var unitEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.UnitChange:
		return true
	case cache.RemoveUnit:
		return true
	}
	return false
}

var branchEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.BranchChange:
		return true
	case cache.RemoveBranch:
		return true
	}
	return false
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}

// testingMultiwatcher is a wrapper for multiwatcher that satisfies the
// modelcache.BackingWatcher interface.
// It allows us to test watcher failure scenarios and manipulate the deltas.
type testingMultiwatcher struct {
	*state.Multiwatcher

	// manipulate gives us the opportunity of manipulating the result of a call
	// to the multi-watcher's "Next" method.
	manipulate func([]multiwatcher.Delta) ([]multiwatcher.Delta, error)
}

func (w testingMultiwatcher) Next() ([]multiwatcher.Delta, error) {
	delta, err := w.Multiwatcher.Next()
	if err == nil && w.manipulate != nil {
		return w.manipulate(delta)
	}
	return delta, err
}
