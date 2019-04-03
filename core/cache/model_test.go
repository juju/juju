// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type ModelSuite struct {
	entitySuite
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

func (s *ModelSuite) TestReport(c *gc.C) {
	m := s.newModel(modelChange)
	c.Assert(m.Report(), jc.DeepEquals, map[string]interface{}{
		"name":              "model-owner/test-model",
		"life":              life.Value("alive"),
		"application-count": 0,
		"charm-count":       0,
		"machine-count":     0,
		"unit-count":        0,
	})
}

func (s *ModelSuite) TestConfig(c *gc.C) {
	m := s.newModel(modelChange)
	c.Assert(m.Config(), jc.DeepEquals, map[string]interface{}{
		"key":     "value",
		"another": "foo",
	})
}

func (s *ModelSuite) TestNewModelGeneratesHash(c *gc.C) {
	s.newModel(modelChange)
	c.Check(testutil.ToFloat64(s.gauges.ModelHashCacheMiss), gc.Equals, float64(1))
}

func (s *ModelSuite) TestModelConfigIncrementsReadCount(c *gc.C) {
	m := s.newModel(modelChange)
	c.Check(testutil.ToFloat64(s.gauges.ModelConfigReads), gc.Equals, float64(0))
	m.Config()
	c.Check(testutil.ToFloat64(s.gauges.ModelConfigReads), gc.Equals, float64(1))
	m.Config()
	c.Check(testutil.ToFloat64(s.gauges.ModelConfigReads), gc.Equals, float64(2))
}

// Some of the tested behaviour in the following methods is specific to the
// watcher, but using a cached model avoids the need to put scaffolding code in
// export_test.go to create a watcher in isolation.
func (s *ModelSuite) TestConfigWatcherStops(c *gc.C) {
	m := s.newModel(modelChange)
	w := m.WatchConfig()
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	wc.AssertStops()
}

func (s *ModelSuite) TestConfigWatcherChange(c *gc.C) {
	m := s.newModel(modelChange)
	w := m.WatchConfig()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key": "changed",
	}

	m.SetDetails(change)
	wc.AssertOneChange()

	// The hash is generated each time we set the details.
	c.Check(testutil.ToFloat64(s.gauges.ModelHashCacheMiss), gc.Equals, float64(2))

	// The value is retrieved from the cache when the watcher is created and notified.
	c.Check(testutil.ToFloat64(s.gauges.ModelHashCacheHit), gc.Equals, float64(2))
}

func (s *ModelSuite) TestConfigWatcherOneValue(c *gc.C) {
	m := s.newModel(modelChange)
	w := m.WatchConfig("key")
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key":     "changed",
		"another": "foo",
	}

	m.SetDetails(change)
	wc.AssertOneChange()
}

func (s *ModelSuite) TestConfigWatcherOneValueOtherChange(c *gc.C) {
	m := s.newModel(modelChange)
	w := m.WatchConfig("key")
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key":     "value",
		"another": "changed",
	}

	m.SetDetails(change)
	wc.AssertNoChange()
}

func (s *ModelSuite) TestConfigWatcherSameValuesCacheHit(c *gc.C) {
	m := s.newModel(modelChange)

	w := m.WatchConfig("key", "another")
	defer workertest.CleanKill(c, w)

	w2 := m.WatchConfig("another", "key")
	defer workertest.CleanKill(c, w2)

	// One cache miss for the "all" hash, and one for the specific fields.
	c.Check(testutil.ToFloat64(s.gauges.ModelHashCacheMiss), gc.Equals, float64(2))

	// Specific field hash should get a hit despite the field ordering.
	c.Check(testutil.ToFloat64(s.gauges.ModelHashCacheHit), gc.Equals, float64(1))
}

func (s *ModelSuite) TestApplicationNotFoundError(c *gc.C) {
	m := s.newModel(modelChange)
	_, err := m.Application("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestCharmNotFoundError(c *gc.C) {
	m := s.newModel(modelChange)
	_, err := m.Charm("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestMachineNotFoundError(c *gc.C) {
	m := s.newModel(modelChange)
	_, err := m.Machine("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestUnitNotFoundError(c *gc.C) {
	m := s.newModel(modelChange)
	_, err := m.Unit("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ControllerSuite) TestWatchMachineStops(c *gc.C) {
	controller, _ := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)

	w := m.WatchMachines()
	wc := NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})
	wc.AssertStops()
}

func (s *ControllerSuite) TestWatchMachineAddMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.processChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineRemoveMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.RemoveMachine{
		ModelUUID: modelChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.processChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineChangeMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "0",
	}
	s.processChange(c, change, events)
	wc.AssertNoChange()
}

func (s *ControllerSuite) TestWatchMachineGatherMachines(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.processChange(c, change, events)
	change2 := change
	change2.Id = "3"
	s.processChange(c, change2, events)
	wc.AssertMaybeCombinedChanges([]string{change.Id, change2.Id})
}

func (s *ControllerSuite) newWithMachine(c *gc.C) (*cache.Controller, <-chan interface{}) {
	events := s.captureEvents(c)
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	s.processChange(c, modelChange, events)
	s.processChange(c, machineChange, events)
	return controller, events
}

func (s *ControllerSuite) setupWithWatchMachine(c *gc.C) (*cache.PredicateStringsWatcher, <-chan interface{}) {
	controller, events := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)

	w := m.WatchMachines()
	return w, events
}

var modelChange = cache.ModelChange{
	ModelUUID: "model-uuid",
	Name:      "test-model",
	Life:      life.Alive,
	Owner:     "model-owner",
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},

	Status: status.StatusInfo{
		Status: status.Active,
	},
}
