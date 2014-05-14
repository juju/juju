// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
)

type serviceSuite struct {
	uniterSuite

	apiService *uniter.Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	var err error
	s.apiService, err = s.uniter.Service(s.wordpressService.Tag())
	c.Assert(err, gc.IsNil)
}

func (s *serviceSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *serviceSuite) TestNameAndString(c *gc.C) {
	c.Assert(s.apiService.Name(), gc.Equals, s.wordpressService.Name())
	c.Assert(s.apiService.String(), gc.Equals, s.wordpressService.String())
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
	err = s.wordpressService.SetExposed()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Destroy the service and check it's detected.
	err = s.wordpressService.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *serviceSuite) TestWatchRelations(c *gc.C) {
	w, err := s.apiService.WatchRelations()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.wordpressService.SetExposed()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add another service and relate it to wordpress,
	// check it's detected.
	s.addMachineServiceCharmAndUnit(c, "mysql")
	rel := s.addRelation(c, "wordpress", "mysql")
	wc.AssertChange(rel.String())

	// Destroy the relation and check it's detected.
	err = rel.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(rel.String())
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *serviceSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err := s.wordpressService.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err = s.apiService.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Dying)
}

func (s *serviceSuite) TestCharmURL(c *gc.C) {
	// Get the charm URL through state calls.
	curl, force := s.wordpressService.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)

	// Now check the same through the API.
	curl, force, err := s.apiService.CharmURL()
	c.Assert(err, gc.IsNil)
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)
}

func (s *serviceSuite) TestGetOwnerTag(c *gc.C) {
	tag, err := s.apiService.GetOwnerTag()
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "user-admin")
}
