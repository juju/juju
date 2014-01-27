// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/firewaller"
	"launchpad.net/juju-core/state/api/params"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type serviceSuite struct {
	firewallerSuite

	apiService *firewaller.Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	apiUnit, err := s.firewaller.Unit(s.units[0].Tag())
	s.apiService, err = apiUnit.Service()
	c.Assert(err, gc.IsNil)
}

func (s *serviceSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *serviceSuite) TestName(c *gc.C) {
	c.Assert(s.apiService.Name(), gc.Equals, s.service.Name())
}

func (s *serviceSuite) TestWatch(c *gc.C) {
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	w, err := s.apiService.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.service.SetExposed()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Destroy the service and check it's detected.
	err = s.service.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *serviceSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err := s.service.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err = s.apiService.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Dying)
}

func (s *serviceSuite) TestIsExposed(c *gc.C) {
	err := s.service.SetExposed()
	c.Assert(err, gc.IsNil)

	isExposed, err := s.apiService.IsExposed()
	c.Assert(err, gc.IsNil)
	c.Assert(isExposed, jc.IsTrue)

	err = s.service.ClearExposed()
	c.Assert(err, gc.IsNil)

	isExposed, err = s.apiService.IsExposed()
	c.Assert(err, gc.IsNil)
	c.Assert(isExposed, jc.IsFalse)
}
