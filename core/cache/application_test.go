// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"sync"

	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type ApplicationSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) TestConfigIncrementsReadCount(c *gc.C) {
	m := s.NewApplication(appChange)
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationConfigReads), gc.Equals, float64(0))

	m.Config()
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationConfigReads), gc.Equals, float64(1))
	m.Config()
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationConfigReads), gc.Equals, float64(2))

	// Goroutine safety.
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			m.Config()
			wg.Done()
		}()
	}
	wg.Wait()
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationConfigReads), gc.Equals, float64(5))
}

// See model_test.go for other config watcher tests.
// Here we just check that WatchConfig is wired up properly.
func (s *ApplicationSuite) TestConfigWatcherChange(c *gc.C) {
	a := s.NewApplication(appChange)
	w := a.WatchConfig()

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, a.Resident, resourceId, true)
	defer func() {
		workertest.CleanKill(c, w)
		s.AssertWorkerResource(c, a.Resident, resourceId, false)
	}()

	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := appChange
	change.Config = map[string]interface{}{"key": "changed"}
	a.SetDetails(change)
	wc.AssertOneChange()

	// The hash is generated each time we set the details.
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationHashCacheMiss), gc.Equals, float64(2))

	// The value is retrieved from the cache when the watcher is created and notified.
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationHashCacheHit), gc.Equals, float64(2))

	// Setting the same values causes no notification and no cache miss.
	a.SetDetails(change)
	wc.AssertNoChange()
	c.Check(testutil.ToFloat64(s.Gauges.ApplicationHashCacheMiss), gc.Equals, float64(2))
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
