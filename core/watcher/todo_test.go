// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

type todoWatcherSuite struct{}

var _ = gc.Suite(&todoWatcherSuite{})

func (s *todoWatcherSuite) TestStringsWatcher(c *gc.C) {
	sw := watcher.TODO[[]string]()
	c.Assert(sw, gc.NotNil)

	swC := watchertest.NewStringsWatcherC(c, sw)
	swC.AssertOneChange()
	swC.AssertNoChange()
	swC.AssertKilled()
}

func (s *todoWatcherSuite) TestNotifyWatcher(c *gc.C) {
	nw := watcher.TODO[struct{}]()
	c.Assert(nw, gc.NotNil)

	nwC := watchertest.NewNotifyWatcherC(c, nw)
	nwC.AssertOneChange()
	nwC.AssertNoChange()
	nwC.AssertKilled()
}
