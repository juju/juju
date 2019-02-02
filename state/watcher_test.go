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
	testing.StateSuite
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
