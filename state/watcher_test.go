// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&watcherSuite{})

type watcherSuite struct {
	ConnSuite
}

func (s *watcherSuite) TestEntityWatcherEventsNonExistent(c *gc.C) {
	// Just watching a document should not trigger an event
	c.Logf("starting watcher for %q %q", "machines", "2")
	w := state.NewEntityWatcher(s.State, "machines", "2")
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
}

func (s *watcherSuite) TestEntityWatcherFirstEvent(c *gc.C) {
	m, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("18.04"), status.NoopStatusHistoryRecorder, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Send the Machine creation event before we start our watcher
	w := m.Watch()

	// This code is essentially what's in NewNotifyWatcherC.AssertOneChange()
	// but we allow for an optional second event. The 2 events are:
	// - initial watcher event
	// - machine created event
	// Due to mixed use of wall clock and test clock in the various bits
	// of infrastructure, we can't guarantee that the watcher fires after
	// the machine created event arrives, which means that event sometimes
	// arrives separately and would fail the test.
	eventCount := 0
loop:
	for {
		select {
		case _, ok := <-w.Changes():
			c.Logf("got change")
			c.Assert(ok, jc.IsTrue)
			eventCount++
			if eventCount > 2 {
				c.Fatalf("watcher sent unexpected change")
			}
		case <-time.After(coretesting.ShortWait):
			if eventCount > 0 {
				break loop
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher did not send change")
			break loop
		}
	}
}

func (s *watcherSuite) TestLegacyActionNotificationWatcher(c *gc.C) {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := dummy.AddUnit(state.AddUnitParams{}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)

	w := state.NewActionNotificationWatcher(s.State, true, unit)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.Model.AddAction(unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(action.Id())

	_, err = action.Cancel()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}
