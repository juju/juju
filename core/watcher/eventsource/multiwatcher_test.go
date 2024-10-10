// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
)

type multiWatcherSuite struct{}

var _ = gc.Suite(&multiWatcherSuite{})

func (*multiWatcherSuite) TestNotifyMultiWatcher(c *gc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiNotifyWatcher(context.Background(), w0, w1)
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer workertest.DirtyKill(c, w)
	wc.AssertOneChange()

	ch0 <- struct{}{}
	ch1 <- struct{}{}
	wc.AssertNChanges(2)

	ch0 <- struct{}{}
	ch1 <- struct{}{}
	wc.AssertAtLeastOneChange()

	workertest.CleanKill(c, w)
}

func (*multiWatcherSuite) TestStringsMultiWatcher(c *gc.C) {
	ch0 := make(chan []string, 1)
	w0 := watchertest.NewMockStringsWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan []string, 1)
	w1 := watchertest.NewMockStringsWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- []string{}
	ch1 <- []string{}

	w, err := NewMultiStringsWatcher(context.Background(), w0, w1)
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewStringsWatcherC(c, w)
	defer workertest.DirtyKill(c, w)

	wc.AssertChange()

	ch0 <- []string{"a", "b"}
	ch1 <- []string{"c", "d"}

	wc.AssertChange("a", "b", "c", "d")

	ch0 <- []string{"e"}
	wc.AssertChange("e")
	ch1 <- []string{"f"}
	wc.AssertChange("f")

	ch0 <- []string{"g"}
	ch1 <- []string{"h"}
	wc.AssertAtLeastOneChange()

	workertest.CleanKill(c, w)
}

func (*multiWatcherSuite) TestMultiWatcherStop(c *gc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiNotifyWatcher(context.Background(), w0, w1)
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer workertest.DirtyKill(c, w)
	wc.AssertOneChange()

	wc.AssertKilled()

	// Ensure that the underlying watchers are also stopped.
	wc0 := watchertest.NewNotifyWatcherC(c, w0)
	wc0.AssertKilled()

	wc1 := watchertest.NewNotifyWatcherC(c, w1)
	wc1.AssertKilled()
}
