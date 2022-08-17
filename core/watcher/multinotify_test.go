// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

type multiNotifyWatcherSuite struct{}

var _ = gc.Suite(&multiNotifyWatcherSuite{})

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcher(c *gc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w := watcher.NewMultiNotifyWatcher(w0, w1)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertKilled()
	wc.AssertOneChange()

	ch0 <- struct{}{}
	wc.AssertOneChange()
	ch1 <- struct{}{}
	wc.AssertOneChange()

	ch0 <- struct{}{}
	ch1 <- struct{}{}
	wc.AssertOneChange()
}
