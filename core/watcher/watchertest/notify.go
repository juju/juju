// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher"
)

type MockNotifyWatcher struct {
	tomb tomb.Tomb
	ch   <-chan struct{}
}

func NewMockNotifyWatcher(ch <-chan struct{}) *MockNotifyWatcher {
	w := &MockNotifyWatcher{ch: ch}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *MockNotifyWatcher) Changes() <-chan struct{} {
	return w.ch
}

func (w *MockNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MockNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

// KillErr can be used to kill the worker with
// an error, to simulate a failing watcher.
func (w *MockNotifyWatcher) KillErr(err error) {
	w.tomb.Kill(err)
}

func (w *MockNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *MockNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func NewNotifyWatcherC(c *tc.C, watcher watcher.NotifyWatcher) NotifyWatcherC {
	return NotifyWatcherC{
		C:       c,
		Watcher: watcher,
	}
}

type NotifyWatcherC struct {
	*tc.C
	Watcher watcher.NotifyWatcher
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c NotifyWatcherC) AssertOneChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertAtLeastOneChange fails if no change is sent before a long time has
// passed.
func (c NotifyWatcherC) AssertAtLeastOneChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

// AssertChanges asserts that there was a series of changes for a given
// duration. If there are any more changes after that period, then it
// will fail.
func (c NotifyWatcherC) AssertChanges(duration time.Duration) {
	if duration >= testing.LongWait {
		c.Fatalf("duration must be less than testing.LongWait")
	}

	done := time.After(duration)
	for {
		select {
		case _, ok := <-c.Watcher.Changes():
			c.Check(ok, jc.IsTrue)
		case <-done:
			// Ensure we have no more changes after we've waited
			// for a given time.
			c.AssertNoChange()
			return
		case <-time.After(testing.LongWait):
			c.Fatalf("watcher did not send a change")
		}
	}
}

// AssertNChanges fails if it does not receive n changes before a long time has passed.
func (c NotifyWatcherC) AssertNChanges(n int) {
	if n <= 1 {
		c.Fatalf("n must be greater than 1")
	}
	received := 0
	for {
		select {
		case _, ok := <-c.Watcher.Changes():
			c.Check(ok, jc.IsTrue)
			received++

			if received < n {
				continue
			}
			// Ensure we have no more changes.
			c.AssertNoChange()
			return
		case <-time.After(testing.LongWait):
			if received == 0 {
				c.Fatalf("watcher did not send any changes")
			} else {
				c.Fatalf("watcher received %d changes, expected %d", received, n)
			}
		}
	}
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c NotifyWatcherC) AssertNoChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	case <-time.After(testing.ShortWait):
	}
}

func (c NotifyWatcherC) assertStops(changesClosed bool) {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case _, ok := <-c.Watcher.Changes():
		if ok || !changesClosed {
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		}
	default:
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes remains open but
// no values are being sent.
func (c NotifyWatcherC) AssertStops() {
	c.assertStops(false)
}

// AssertKilled Kills the watcher and asserts that Wait completes without
// error before a long time has passed.
func (c NotifyWatcherC) AssertKilled() {
	c.assertStops(true)
}
