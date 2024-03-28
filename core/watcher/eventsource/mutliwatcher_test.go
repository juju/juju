// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
)

type multiNotifyWatcherSuite struct{}

var _ = gc.Suite(&multiNotifyWatcherSuite{})

func (*multiNotifyWatcherSuite) TestMultiWatcher(c *gc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiWatcher[struct{}](context.Background(), w0, w1)
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer workertest.DirtyKill(c, w)
	wc.AssertOneChange()

	ch0 <- struct{}{}
	wc.AssertOneChange()
	ch1 <- struct{}{}
	wc.AssertOneChange()

	ch0 <- struct{}{}
	ch1 <- struct{}{}
	wc.AssertOneChange()

	workertest.CleanKill(c, w)
}

func (*multiNotifyWatcherSuite) TestMultiWatcherStop(c *gc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiWatcher[struct{}](context.Background(), w0, w1)
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer workertest.DirtyKill(c, w)
	wc.AssertOneChange()

	workertest.CleanKill(c, w)
	wc.AssertKilled()

	// Ensure that the underlying watchers are also stopped.
	wc0 := watchertest.NewNotifyWatcherC(c, w0)
	wc0.AssertKilled()

	wc1 := watchertest.NewNotifyWatcherC(c, w1)
	wc1.AssertKilled()
}
