// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
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

func (s *ApplicationSuite) status(value status.Status, when time.Time) status.StatusInfo {
	return status.StatusInfo{
		Status: value,
		Since:  &when,
	}
}

func (s *ApplicationSuite) TestStatusWhenSet(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	appStatus := s.status(status.Active, time.Now())
	model.UpdateApplication(cache.ApplicationChange{
		Name:   "app",
		Status: appStatus,
	}, s.Manager)
	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Status(), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestStatusWhenUnsetNoUnits(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	now := time.Now()
	model.UpdateApplication(cache.ApplicationChange{
		Name:   "app",
		Status: s.status(status.Unset, now),
	}, s.Manager)
	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Status(), jc.DeepEquals, status.StatusInfo{
		Status: status.Unknown,
		Since:  &now,
	})
}

func (s *ApplicationSuite) TestStatusWhenUnsetWithUnits(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	model.UpdateApplication(cache.ApplicationChange{
		Name:   "app",
		Status: s.status(status.Unset, time.Now()),
	}, s.Manager)
	model.UpdateUnit(cache.UnitChange{
		Name:           "app/1",
		Application:    "app",
		WorkloadStatus: s.status(status.Active, time.Now()),
	}, s.Manager)
	// Application status derivation uses the status.DeriveStatus method
	// which defines the relative priorities of the status values.
	expected := s.status(status.Waiting, time.Now().Add(-time.Minute))
	model.UpdateUnit(cache.UnitChange{
		Name:           "app/2",
		Application:    "app",
		WorkloadStatus: expected,
	}, s.Manager)

	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Status(), jc.DeepEquals, expected)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorRunning(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	appStatus := s.status(status.Active, time.Now())
	model.UpdateApplication(cache.ApplicationChange{
		Name:           "app",
		Status:         appStatus,
		OperatorStatus: s.status(status.Running, time.Now()),
	}, s.Manager)
	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.DisplayStatus(), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorActive(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	appStatus := s.status(status.Blocked, time.Now())
	model.UpdateApplication(cache.ApplicationChange{
		Name:           "app",
		Status:         appStatus,
		OperatorStatus: s.status(status.Active, time.Now()),
	}, s.Manager)
	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.DisplayStatus(), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorWaiting(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	expected := s.status(status.Waiting, time.Now())
	expected.Message = status.MessageInstallingAgent
	model.UpdateApplication(cache.ApplicationChange{
		Name:           "app",
		Status:         s.status(status.Active, time.Now()),
		OperatorStatus: expected}, s.Manager)
	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.DisplayStatus(), jc.DeepEquals, expected)
}

func (s *ApplicationSuite) TestUnitsSorted(c *gc.C) {
	model := s.NewModel(cache.ModelChange{
		Name: "test",
	})
	model.UpdateApplication(cache.ApplicationChange{
		Name:   "app",
		Status: s.status(status.Unset, time.Now()),
	}, s.Manager)
	model.UpdateUnit(cache.UnitChange{
		Name:        "app/1",
		Application: "app",
	}, s.Manager)
	model.UpdateUnit(cache.UnitChange{
		Name:        "app/2",
		Application: "app",
	}, s.Manager)
	model.UpdateUnit(cache.UnitChange{
		Name:        "app/10",
		Application: "app",
	}, s.Manager)

	app, err := model.Application("app")
	c.Assert(err, jc.ErrorIsNil)
	units := app.Units()

	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	// Simple alphabetical sort for now as we may well soon have unit IDs that
	// are hashes.
	c.Assert(names, jc.DeepEquals, []string{"app/1", "app/10", "app/2"})
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
