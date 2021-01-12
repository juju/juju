// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&watcherSuite{})

type watcherSuite struct {
	ConnSuite
}

func (s *watcherSuite) TestEntityWatcherEventsNonExistent(c *gc.C) {
	// Just watching a document should not trigger an event
	c.Logf("starting watcher for %q %q", "machines", "2")
	w := state.NewEntityWatcher(s.State, "machines", "2")
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestEntityWatcherFirstEvent(c *gc.C) {
	m, err := s.State.AddMachine("bionic", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Send the Machine creation event before we start our watcher
	s.State.StartSync()
	w := m.Watch()
	c.Logf("Watch started")
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestLegacyActionNotificationWatcher(c *gc.C) {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := dummy.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	w := state.NewActionNotificationWatcher(s.State, true, unit)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()

	operationID, err := s.Model.EnqueueOperation("a test")
	c.Assert(err, jc.ErrorIsNil)
	action, err := unit.AddAction(operationID, "snapshot", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(action.Id())

	_, err = action.Cancel()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}
