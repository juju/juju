// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher/watchertest"
)

type applicationSuite struct {
	firewallerSuite

	apiApplication *firewaller.Application
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	apiUnit, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
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
	c.Assert(s.apiApplication.Life(), gc.Equals, params.Alive)

	w, err := s.apiApplication.Watch()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.application.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Destroy the application and check it's detected.
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *applicationSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiApplication.Life(), gc.Equals, params.Alive)

	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiApplication.Life(), gc.Equals, params.Alive)

	err = s.apiApplication.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiApplication.Life(), gc.Equals, params.Dying)
}

func (s *applicationSuite) TestIsExposed(c *gc.C) {
	err := s.application.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err := s.apiApplication.IsExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isExposed, jc.IsTrue)

	err = s.application.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	isExposed, err = s.apiApplication.IsExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isExposed, jc.IsFalse)
}
