// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

type normaliseWatcherSuite struct{}

func TestNormaliseWatcherSuite(t *testing.T) {
	tc.Run(t, &normaliseWatcherSuite{})
}

func (s *normaliseWatcherSuite) TestStringsWatcher(c *tc.C) {
	ch := make(chan []string, 1)
	source := watchertest.NewMockStringsWatcher(ch)

	nw, err := watcher.Normalise[[]string](source)
	c.Assert(err, tc.ErrorIsNil)

	nwC := watchertest.NewNotifyWatcherC(c, nw)

	ch <- []string{}
	nwC.AssertOneChange()

	ch <- []string{"does", "not", "matter"}
	nwC.AssertOneChange()

	nwC.AssertNoChange()

	nwC.AssertKilled()
}

func (s *normaliseWatcherSuite) TestSourceDies(c *tc.C) {
	ch := make(chan []string, 1)
	source := watchertest.NewMockStringsWatcher(ch)

	nw, err := watcher.Normalise[[]string](source)
	c.Assert(err, tc.ErrorIsNil)

	nwC := watchertest.NewNotifyWatcherC(c, nw)

	ch <- []string{}
	nwC.AssertOneChange()

	ch <- []string{"does", "not", "matter"}
	nwC.AssertOneChange()

	nwC.AssertNoChange()

	workertest.CleanKill(c, source)
	close(ch)
	workertest.CheckKilled(c, nw)
}

func (s *normaliseWatcherSuite) TestNotifyWatcherElided(c *tc.C) {
	ch := make(chan struct{}, 1)
	source := watchertest.NewMockNotifyWatcher(ch)

	nw, err := watcher.Normalise[struct{}](source)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nw, tc.Equals, source)
}
