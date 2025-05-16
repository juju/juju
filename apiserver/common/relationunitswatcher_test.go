// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationUnitsWatcherSuite struct{}

func TestRelationUnitsWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &relationUnitsWatcherSuite{})
}
func (s *relationUnitsWatcherSuite) TestRelationUnitsWatcherFromDomain(c *tc.C) {

	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromDomain(source)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(source.Err(), tc.Equals, tomb.ErrStillAlive)

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
		c.Assert(result, tc.DeepEquals, params.RelationUnitsChange{
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

	c.Assert(worker.Stop(w), tc.ErrorIsNil)
	// Ensure that stopping the watcher has stopped the source.
	c.Assert(source.Err(), tc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.Equals, false)
	default:
		c.Fatalf("didn't close output channel")
	}
}

func (s *relationUnitsWatcherSuite) TestCanStopWithAPendingSend(c *tc.C) {
	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromDomain(source)
	c.Assert(err, tc.ErrorIsNil)
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
		err := worker.Stop(w)
		stopped <- err
	}()

	select {
	case err := <-stopped:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for watcher to stop with pending send")
	}
}

func (s *relationUnitsWatcherSuite) TestNilChanged(c *tc.C) {
	source := &mockRUWatcher{
		changes: make(chan watcher.RelationUnitsChange),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	w, err := common.RelationUnitsWatcherFromDomain(source)
	c.Assert(err, tc.ErrorIsNil)

	event := watcher.RelationUnitsChange{
		Departed: []string{"happy", "birthday"},
	}

	s.send(c, source.changes, event)
	result := s.receive(c, w)
	c.Assert(result, tc.DeepEquals, params.RelationUnitsChange{
		Departed: []string{"happy", "birthday"},
	})
}

func (s *relationUnitsWatcherSuite) send(c *tc.C, ch chan watcher.RelationUnitsChange, event watcher.RelationUnitsChange) {
	select {
	case ch <- event:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting to send event")
	}
}

func (s *relationUnitsWatcherSuite) receive(c *tc.C, w common.RelationUnitsWatcher) params.RelationUnitsChange {
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
