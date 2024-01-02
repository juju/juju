// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type applicationSuite struct {
	firewallerSuite

	apiApplication *firewaller.Application
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	apiUnit, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	s.apiApplication, err = apiUnit.Application()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *applicationSuite) TestName(c *gc.C) {
	c.Assert(s.apiApplication.Name(), gc.Equals, s.application.Name())
}

func (s *applicationSuite) TestTag(c *gc.C) {
	c.Assert(s.apiApplication.Tag(), gc.Equals, names.NewApplicationTag(s.application.Name()))
}

func (s *applicationSuite) TestWatch(c *gc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w, err := s.apiApplication.Watch()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.application.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Destroy the application and check it's detected.
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *applicationSuite) TestExposeInfo(c *gc.C) {
	err := s.application.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"10.0.0.0/16", "192.168.0.0/24"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	isExposed, exposedEndpoints, err := s.apiApplication.ExposeInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isExposed, jc.IsTrue)
	c.Assert(exposedEndpoints, gc.DeepEquals, map[string]params.ExposedEndpoint{
		"": {
			ExposeToSpaces: []string{network.AlphaSpaceId},
			ExposeToCIDRs:  []string{"10.0.0.0/16", "192.168.0.0/24"},
		},
	})

	err = s.application.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	isExposed, exposedEndpoints, err = s.apiApplication.ExposeInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isExposed, jc.IsFalse)
	c.Assert(exposedEndpoints, gc.HasLen, 0)
}
