// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type ModelSuite struct {
	testing.IsolationSuite

	gauges *cache.ControllerGauges

	hub *pubsub.SimpleHub
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.gauges = cache.CreateControllerGauges()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	s.hub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		Logger: logger,
	})

}

func (s *ModelSuite) newModel(details cache.ModelChange) *cache.Model {
	m := cache.NewModel(s.gauges, s.hub)
	m.SetDetails(details)
	return m
}

func (s *ModelSuite) TestReport(c *gc.C) {
	m := s.newModel(modelChange)
	c.Assert(m.Report(), jc.DeepEquals, map[string]interface{}{
		"name": "model-owner/test-model",
		"life": life.Value("alive"),
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
