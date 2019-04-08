// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type ApplicationSuite struct {
	entitySuite
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

func (s *ApplicationSuite) TestConfigIncrementsReadCount(c *gc.C) {
	m := s.newApplication(appChange)
	c.Check(testutil.ToFloat64(s.gauges.ApplicationConfigReads), gc.Equals, float64(0))
	m.Config()
	c.Check(testutil.ToFloat64(s.gauges.ApplicationConfigReads), gc.Equals, float64(1))
	m.Config()
	c.Check(testutil.ToFloat64(s.gauges.ApplicationConfigReads), gc.Equals, float64(2))
}

// See model_test.go for other config watcher tests.
// Here we just check that WatchConfig is wired up properly.
func (s *ApplicationSuite) TestConfigWatcherChange(c *gc.C) {
	a := s.newApplication(appChange)
	w := a.WatchConfig()
	defer workertest.CleanKill(c, w)

	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := appChange
	change.Config = map[string]interface{}{"key": "changed"}
	a.SetDetails(change)
	wc.AssertOneChange()

	// The hash is generated each time we set the details.
	c.Check(testutil.ToFloat64(s.gauges.ApplicationHashCacheMiss), gc.Equals, float64(2))

	// The value is retrieved from the cache when the watcher is created and notified.
	c.Check(testutil.ToFloat64(s.gauges.ApplicationHashCacheHit), gc.Equals, float64(2))
}

func (s *ApplicationSuite) newApplication(details cache.ApplicationChange) *cache.Application {
	a := cache.NewApplication(s.gauges, s.hub)
	a.SetDetails(details)
	return a
}

var appChange = cache.ApplicationChange{
	ModelUUID:   "model-uuid",
	Name:        "application-name",
	Exposed:     false,
	CharmURL:    "www.charm-url.com-1",
	Life:        life.Alive,
	MinUnits:    0,
	Constraints: constraints.Value{},
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},
	Subordinate:     false,
	Status:          status.StatusInfo{Status: status.Active},
	WorkloadVersion: "666",
}

var removeApp = cache.RemoveApplication{
	ModelUUID: "model-uuid",
	Name:      "application-name",
}
