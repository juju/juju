// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	tomb "gopkg.in/tomb.v2"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
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

func (w *MockNotifyWatcher) Changes() watcher.NotifyChannel {
	return watcher.NotifyChannel(w.ch)
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

func NewNotifyWatcherC(c *gc.C, watcher watcher.NotifyWatcher, preAssert func()) NotifyWatcherC {
	if preAssert == nil {
		preAssert = func() {}
	}
	return NotifyWatcherC{
		C:         c,
		Watcher:   watcher,
		PreAssert: preAssert,
	}
}

type NotifyWatcherC struct {
	*gc.C
	Watcher   watcher.NotifyWatcher
	PreAssert func()
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c NotifyWatcherC) AssertOneChange() {
	c.PreAssert()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c NotifyWatcherC) AssertNoChange() {
	c.PreAssert()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes remains open but
// no values are being sent.
func (c NotifyWatcherC) AssertStops() {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		c.PreAssert()
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	c.PreAssert()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	default:
	}
}
