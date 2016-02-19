// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher/watchertest"
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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestNameTagAndString(c *gc.C) {
	c.Assert(s.apiService.Name(), gc.Equals, s.wordpressService.Name())
	c.Assert(s.apiService.String(), gc.Equals, s.wordpressService.String())
	c.Assert(s.apiService.Tag(), gc.Equals, s.wordpressService.Tag().(names.ServiceTag))
}

func (s *serviceSuite) TestWatch(c *gc.C) {
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	w, err := s.apiService.Watch()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.wordpressService.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Destroy the service and check it's detected.
	err = s.wordpressService.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *serviceSuite) TestWatchRelations(c *gc.C) {
	w, err := s.apiService.WatchRelations()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.wordpressService.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add another service and relate it to wordpress,
	// check it's detected.
	s.addMachineServiceCharmAndUnit(c, "mysql")
	rel := s.addRelation(c, "wordpress", "mysql")
	wc.AssertChange(rel.String())

	// Destroy the relation and check it's detected.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel.String())
	wc.AssertNoChange()
}

func (s *serviceSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err := s.wordpressService.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Alive)

	err = s.apiService.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiService.Life(), gc.Equals, params.Dying)
}

func (s *serviceSuite) TestCharmURL(c *gc.C) {
	// Get the charm URL through state calls.
	curl, force := s.wordpressService.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)

	// Now check the same through the API.
	curl, force, err := s.apiService.CharmURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)
}

func (s *serviceSuite) TestCharmModifiedVersion(c *gc.C) {
	// Get the charm URL through state calls.
	ver, err := s.apiService.CharmModifiedVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Equals, s.wordpressService.CharmModifiedVersion())
}

func (s *serviceSuite) TestOwnerTag(c *gc.C) {
	tag, err := s.apiService.OwnerTag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, s.AdminUserTag(c))
}

func (s *serviceSuite) TestSetServiceStatus(c *gc.C) {
	message := "a test message"
	stat, err := s.wordpressService.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Status, gc.Not(gc.Equals), state.Status(params.StatusActive))
	c.Assert(stat.Message, gc.Not(gc.Equals), message)

	err = s.apiService.SetStatus(s.wordpressUnit.Name(), params.StatusActive, message, map[string]interface{}{})
	c.Check(err, gc.ErrorMatches, `"wordpress/0" is not leader of "wordpress"`)

	s.claimLeadership(c, s.wordpressUnit, s.wordpressService)

	err = s.apiService.SetStatus(s.wordpressUnit.Name(), params.StatusActive, message, map[string]interface{}{})
	c.Check(err, jc.ErrorIsNil)

	stat, err = s.wordpressService.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(stat.Status, gc.Equals, state.Status(params.StatusActive))
	c.Check(stat.Message, gc.Equals, message)
}

func (s *serviceSuite) TestServiceStatus(c *gc.C) {
	message := "a test message"
	stat, err := s.wordpressService.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Status, gc.Not(gc.Equals), state.Status(params.StatusActive))
	c.Assert(stat.Message, gc.Not(gc.Equals), message)

	err = s.wordpressService.SetStatus(state.Status(params.StatusActive), message, map[string]interface{}{})
	c.Check(err, jc.ErrorIsNil)

	stat, err = s.wordpressService.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(stat.Status, gc.Equals, state.Status(params.StatusActive))
	c.Check(stat.Message, gc.Equals, message)

	result, err := s.apiService.Status(s.wordpressUnit.Name())
	c.Check(err, gc.ErrorMatches, `"wordpress/0" is not leader of "wordpress"`)

	s.claimLeadership(c, s.wordpressUnit, s.wordpressService)
	result, err = s.apiService.Status(s.wordpressUnit.Name())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Service.Status, gc.Equals, params.StatusActive)
}

func (s *serviceSuite) claimLeadership(c *gc.C, unit *state.Unit, service *state.Service) {
	claimer := s.State.LeadershipClaimer()
	err := claimer.ClaimLeadership(service.Name(), unit.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}
