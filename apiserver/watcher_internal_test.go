// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type allWatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&allWatcherSuite{})

func (s *allWatcherSuite) watcher() *SrvAllWatcher {
	// We explicitly don't have a real watcher here as the tests
	// are for the translation of types.
	return &SrvAllWatcher{}
}

func (s *allWatcherSuite) TestTranslateApplicationWithStatus(c *gc.C) {
	w := s.watcher()
	input := &multiwatcher.ApplicationInfo{
		ModelUUID: testing.ModelTag.Id(),
		Name:      "test-app",
		CharmURL:  "test-app",
		Life:      life.Alive,
		Status: multiwatcher.StatusInfo{
			Current: status.Active,
		},
	}
	output := w.translateApplication(input)
	c.Assert(output, jc.DeepEquals, &params.ApplicationInfo{
		ModelUUID: input.ModelUUID,
		Name:      input.Name,
		CharmURL:  input.CharmURL,
		Life:      input.Life,
		Status: params.StatusInfo{
			Current: status.Active,
		},
	})
}

func (s *allWatcherSuite) setupCache(c *gc.C) *cache.Controller {
	changes := make(chan interface{})
	handled := make(chan interface{})
	notify := func(evt interface{}) {
		c.Logf("%#v", evt)
		select {
		case handled <- evt:
		case <-time.After(testing.LongWait):
			c.Fatalf("handled notify not retrieved")
		}
	}
	sendEvent := func(event interface{}) {
		select {
		case changes <- event:
		case <-time.After(testing.LongWait):
			c.Fatal("cache did not accept event")
		}
		select {
		case <-handled:
		case <-time.After(testing.LongWait):
			c.Fatal("cache did not handle event")
		}
	}

	controller, err := cache.NewController(cache.ControllerConfig{
		Changes: changes,
		Notify:  notify,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })

	sendEvent(cache.ModelChange{
		ModelUUID: testing.ModelTag.Id(),
		// Defaults for everything else.
	})
	sendEvent(cache.ApplicationChange{
		ModelUUID: testing.ModelTag.Id(),
		Name:      "test-app",
		Status: status.StatusInfo{
			Status: status.Unset,
		},
		// Defaults for everything else.
	})
	sendEvent(cache.UnitChange{
		ModelUUID:   testing.ModelTag.Id(),
		Name:        "test-app/0",
		Application: "test-app",
		WorkloadStatus: status.StatusInfo{
			Status: status.Active,
		},
		// Defaults for everything else.
	})

	return controller
}

func (s *allWatcherSuite) TestTranslateApplicationStatusUnset(c *gc.C) {
	controller := s.setupCache(c)
	w := s.watcher()
	w.controller = controller
	input := &multiwatcher.ApplicationInfo{
		ModelUUID: testing.ModelTag.Id(),
		Name:      "test-app",
		CharmURL:  "test-app",
		Life:      life.Alive,
		Status: multiwatcher.StatusInfo{
			Current: status.Unset,
		},
	}
	output := w.translateApplication(input)
	c.Assert(output, jc.DeepEquals, &params.ApplicationInfo{
		ModelUUID: input.ModelUUID,
		Name:      input.Name,
		CharmURL:  input.CharmURL,
		Life:      input.Life,
		Status: params.StatusInfo{
			Current: status.Active,
		},
	})
}
