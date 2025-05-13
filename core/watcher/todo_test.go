// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

type todoWatcherSuite struct{}

var _ = tc.Suite(&todoWatcherSuite{})

func (s *todoWatcherSuite) TestStringsWatcher(c *tc.C) {
	sw := watcher.TODO[[]string]()
	c.Assert(sw, tc.NotNil)

	swC := watchertest.NewStringsWatcherC(c, sw)
	swC.AssertOneChange()
	swC.AssertNoChange()
	swC.AssertKilled()
}

func (s *todoWatcherSuite) TestNotifyWatcher(c *tc.C) {
	nw := watcher.TODO[struct{}]()
	c.Assert(nw, tc.NotNil)

	nwC := watchertest.NewNotifyWatcherC(c, nw)
	nwC.AssertOneChange()
	nwC.AssertNoChange()
	nwC.AssertKilled()
}
