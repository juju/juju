// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
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

func (s *ApplicationSuite) TestFieldWatcherStops(c *gc.C) {
	m := s.newApplication(appChange)
	w := m.WatchFields()
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	wc.AssertStops()
}

func (s *ApplicationSuite) TestFieldWatcherAnyChange(c *gc.C) {
	m := s.newApplication(appChange)
	w := m.WatchFields()
	defer workertest.CleanKill(c, w)

	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := appChange
	change.Config = map[string]interface{}{
		"key": "changed",
	}
	m.SetDetails(change)
	wc.AssertOneChange()
}

func (s *ApplicationSuite) TestFieldWatcherSpecificChange(c *gc.C) {
	m := s.newApplication(appChange)
	w := m.WatchFields(cache.ApplicationCharmURLChanged, cache.ApplicationExposedChanged)
	defer workertest.CleanKill(c, w)

	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := appChange

	// Not a change we are watching for.
	change.Config = map[string]interface{}{
		"key": "changed",
	}
	m.SetDetails(change)
	wc.AssertNoChange()

	change.CharmURL = "www.new-charm-url.com"
	m.SetDetails(change)
	wc.AssertOneChange()

	change.Exposed = true
	m.SetDetails(change)
	wc.AssertOneChange()
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
	CharmURL:    "www.charm-url.com",
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
