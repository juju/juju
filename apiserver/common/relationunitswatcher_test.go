// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type relationUnitsWatcherSuite struct{}

var _ = gc.Suite(&relationUnitsWatcherSuite{})

func (s *relationUnitsWatcherSuite) TestRelationUnitsWatcherFromState(c *gc.C) {

	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromState(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(source.Err(), gc.Equals, tomb.ErrStillAlive)
	c.Assert(w.Err(), gc.Equals, tomb.ErrStillAlive)

	event := watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"joni/1": {Version: 23},
		},
		AppChanged: map[string]int64{
			"mitchell": 42,
		},
		Departed: []string{"urge", "for", "going"},
	}
	select {
	case source.changes <- event:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting to send event")
	}

	// The running loop of the watcher is effectively a 1-event
	// buffer.

	select {
	case result := <-w.Changes():
		c.Assert(result, gc.DeepEquals, params.RelationUnitsChange{
			Changed: map[string]params.UnitSettings{
				"joni/1": {Version: 23},
			},
			AppChanged: map[string]int64{
				"mitchell": 42,
			},
			Departed: []string{"urge", "for", "going"},
		})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for output event")
	}

	c.Assert(w.Stop(), jc.ErrorIsNil)
	// Ensure that stopping the watcher has stopped the source.
	c.Assert(source.Err(), jc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, false)
	default:
		c.Fatalf("didn't close output channel")
	}
}

func (s *relationUnitsWatcherSuite) TestCanStopWithAPendingSend(c *gc.C) {
	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromState(source)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Kill()

	event := watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"joni/1": {Version: 23},
		},
	}
	s.send(c, source.changes, event)

	// Stop without accepting the output event.
	stopped := make(chan error)
	go func() {
		err := w.Stop()
		stopped <- err
	}()

	select {
	case err := <-stopped:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for watcher to stop with pending send")
	}
}

func (s *relationUnitsWatcherSuite) TestNilChanged(c *gc.C) {
	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromState(source)
	c.Assert(err, jc.ErrorIsNil)

	event := watcher.RelationUnitsChange{
		Departed: []string{"happy", "birthday"},
	}

	s.send(c, source.changes, event)
	result := s.receive(c, w)
	c.Assert(result, gc.DeepEquals, params.RelationUnitsChange{
		Departed: []string{"happy", "birthday"},
	})
}

func (s *relationUnitsWatcherSuite) send(c *gc.C, ch chan watcher.RelationUnitsChange, event watcher.RelationUnitsChange) {
	select {
	case ch <- event:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting to send event")
	}
}

func (s *relationUnitsWatcherSuite) receive(c *gc.C, w common.RelationUnitsWatcher) params.RelationUnitsChange {
	select {
	case result := <-w.Changes():
		return result
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for output event")
	}
	// Can't actually happen, but the go compiler can't tell that
	// c.Fatalf panics.
	return params.RelationUnitsChange{}
}

type mockRUWatcher struct {
	tomb.Tomb
	changes chan watcher.RelationUnitsChange
}

func (w *mockRUWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}

func (w *mockRUWatcher) Kill() {
	w.Tomb.Kill(nil)
}

func (w *mockRUWatcher) Stop() error {
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

func (w *mockRUWatcher) Err() error {
	return w.Tomb.Err()
}
