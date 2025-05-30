// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/watcher/watchertest"
)

type multiWatcherSuite struct{}

func TestMultiWatcherSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &multiWatcherSuite{})
}

func (*multiWatcherSuite) TestNotifyMultiWatcher(c *tc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiNotifyWatcher(c.Context(), w0, w1)
	c.Assert(err, tc.ErrorIsNil)

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

func (*multiWatcherSuite) TestMultiWatcherStop(c *tc.C) {
	ch0 := make(chan struct{}, 1)
	w0 := watchertest.NewMockNotifyWatcher(ch0)
	defer workertest.DirtyKill(c, w0)

	ch1 := make(chan struct{}, 1)
	w1 := watchertest.NewMockNotifyWatcher(ch1)
	defer workertest.DirtyKill(c, w1)

	// Initial events are consumed by the multiwatcher.
	ch0 <- struct{}{}
	ch1 <- struct{}{}

	w, err := NewMultiNotifyWatcher(c.Context(), w0, w1)
	c.Assert(err, tc.ErrorIsNil)

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
