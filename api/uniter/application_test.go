// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/leadership"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

type applicationSuite struct {
	uniterSuite

	apiApplication *uniter.Application
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	var err error
	s.apiApplication, err = s.uniter.Application(s.wordpressApplication.Tag().(names.ApplicationTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestNameTagAndString(c *gc.C) {
	c.Assert(s.apiApplication.Name(), gc.Equals, s.wordpressApplication.Name())
	c.Assert(s.apiApplication.String(), gc.Equals, s.wordpressApplication.String())
	c.Assert(s.apiApplication.Tag(), gc.Equals, s.wordpressApplication.Tag().(names.ApplicationTag))
}

func (s *applicationSuite) TestWatch(c *gc.C) {
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	c.Assert(s.apiApplication.Life(), gc.Equals, life.Alive)

	w, err := s.apiApplication.Watch()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.wordpressApplication.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Destroy the application and check it's detected.
	err = s.wordpressApplication.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *applicationSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiApplication.Life(), gc.Equals, life.Alive)

	err := s.wordpressApplication.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiApplication.Life(), gc.Equals, life.Alive)

	err = s.apiApplication.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiApplication.Life(), gc.Equals, life.Dying)
}

func (s *applicationSuite) TestCharmURL(c *gc.C) {
	// Get the charm URL through state calls.
	curl, force := s.wordpressApplication.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)

	// Now check the same through the API.
	curl, force, err := s.apiApplication.CharmURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, s.wordpressCharm.URL())
	c.Assert(force, jc.IsFalse)
}

func (s *applicationSuite) TestCharmModifiedVersion(c *gc.C) {
	// Get the charm URL through state calls.
	ver, err := s.apiApplication.CharmModifiedVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Equals, s.wordpressApplication.CharmModifiedVersion())
}

func (s *applicationSuite) TestSetApplicationStatus(c *gc.C) {
	message := "a test message"
	stat, err := s.wordpressApplication.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Status, gc.Not(gc.Equals), status.Active)
	c.Assert(stat.Message, gc.Not(gc.Equals), message)

	err = s.apiApplication.SetStatus(s.wordpressUnit.Name(), status.Active, message, map[string]interface{}{})
	c.Check(err, gc.ErrorMatches, `"wordpress/0" is not leader of "wordpress"`)

	s.claimLeadership(c, s.wordpressUnit, s.wordpressApplication)

	err = s.apiApplication.SetStatus(s.wordpressUnit.Name(), status.Active, message, map[string]interface{}{})
	c.Check(err, jc.ErrorIsNil)

	stat, err = s.wordpressApplication.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(stat.Status, gc.Equals, status.Active)
	c.Check(stat.Message, gc.Equals, message)
}

func (s *applicationSuite) TestApplicationStatus(c *gc.C) {
	message := "a test message"
	stat, err := s.wordpressApplication.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Status, gc.Not(gc.Equals), status.Active)
	c.Assert(stat.Message, gc.Not(gc.Equals), message)

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: message,
		Data:    map[string]interface{}{},
		Since:   &now,
	}
	err = s.wordpressApplication.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	stat, err = s.wordpressApplication.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(stat.Status, gc.Equals, status.Active)
	c.Check(stat.Message, gc.Equals, message)

	result, err := s.apiApplication.Status(s.wordpressUnit.Name())
	c.Check(err, gc.ErrorMatches, `"wordpress/0" is not leader of "wordpress"`)

	s.claimLeadership(c, s.wordpressUnit, s.wordpressApplication)
	result, err = s.apiApplication.Status(s.wordpressUnit.Name())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Error, gc.IsNil)
	c.Check(result.Application.Status, gc.Equals, status.Active.String())
}

func (s *applicationSuite) claimLeadership(c *gc.C, unit *state.Unit, app *state.Application) {
	claimer := leadership.NewClient(s.st)
	err := claimer.ClaimLeadership(app.Name(), unit.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}
