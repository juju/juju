// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"net/url"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	statetesting "github.com/juju/juju/state/testing"
)

type serviceSuite struct {
	uniterSuite

	apiService *uniter.Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	var err error
	s.apiService, err = s.uniter.Service(s.wordpressService.Tag().(names.ServiceTag))
	c.Assert(err, gc.IsNil)
}

func (s *serviceSuite) TestNameTagAndString(c *gc.C) {
	c.Assert(s.apiService.Name(), gc.Equals, s.wordpressService.Name())
	c.Assert(s.apiService.String(), gc.Equals, s.wordpressService.String())
	c.Assert(s.apiService.Tag(), gc.Equals, s.wordpressService.Tag().(names.ServiceTag))
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

func (s *serviceSuite) TestOwnerTagV0(c *gc.C) {
	s.patchNewState(c, uniter.NewStateV0)

	tag, err := s.apiService.OwnerTag()
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, s.AdminUserTag(c))
}

func (s *serviceSuite) TestOwnerTagV1(c *gc.C) {
	s.patchNewState(c, uniter.NewStateV1)

	tag, err := s.apiService.OwnerTag()
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, s.AdminUserTag(c))
}

func (s *serviceSuite) patchNewState(
	c *gc.C,
	patchFunc func(_ base.APICaller, _ names.UnitTag, _ *url.URL) *uniter.State,
) {
	s.uniterSuite.patchNewState(c, patchFunc)
	var err error
	s.apiService, err = s.uniter.Service(s.wordpressService.Tag().(names.ServiceTag))
	c.Assert(err, gc.IsNil)
}
