// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/application"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationUnitsWatcherSuite struct {
	service *MockRelationService
}

func TestRelationUnitsWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &relationUnitsWatcherSuite{})
}

func (s *relationUnitsWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockRelationService(ctrl)

	c.Cleanup(func() {
		s.service = nil
	})

	return ctrl
}

func (s *relationUnitsWatcherSuite) TestRelationUnitsWatcherFromDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Setup mock relation units watcher
	source := &mockRUWatcher{
		changes: make(chan struct{}),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})
	c.Assert(source.Err(), tc.Equals, tomb.ErrStillAlive)

	// Setup the service call at loop start
	appUUID := tc.Must(c, application.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	initialChangeInfo := relation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"joni/0": 75,
		},
		AppSettingsVersion: map[string]int64{
			"mitchell": 42,
		},
		DepartedUnits: []string{"urge", "for"},
	}
	s.service.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(initialChangeInfo, nil)

	// Setup for service call after notify event.
	changeInfo := relation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"joni/0": 75,
			"joni/1": 23,
		},
		AppSettingsVersion: map[string]int64{
			"mitchell": 38,
		},
		DepartedUnits: []string{"urge", "for", "going"},
	}
	s.service.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(changeInfo, nil)

	// Setup the wrapped watcher for test.
	w, err := wrappedRelationChangesWatcher(
		source,
		appUUID,
		relUUID,
		s.service,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Act: send a notify event
	s.send(c, source.changes)

	// Assert
	// The running loop of the watcher is effectively a 1-event
	// buffer.
	obtained := s.receive(c, w)
	c.Check(obtained, tc.DeepEquals, params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"joni/1": {Version: 23},
		},
		AppChanged: map[string]int64{
			"mitchell": 38,
		},
		Departed: []string{"going"},
	})

	c.Check(worker.Stop(w), tc.ErrorIsNil)
	// Ensure that stopping the watcher has stopped the source.
	c.Check(source.Err(), tc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.Equals, false)
	default:
		c.Fatalf("didn't close output channel")
	}
}

func (s *relationUnitsWatcherSuite) TestCanStopWithAPendingSend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Setup mock relation units watcher
	source := &mockRUWatcher{
		changes: make(chan struct{}),
	}
	// Ensure the watcher tomb thinks it's still going.
	source.Tomb.Go(func() error {
		<-source.Tomb.Dying()
		return nil
	})

	// Setup the service call at loop start
	appUUID := tc.Must(c, application.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	s.service.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(relation.ConsumerRelationUnitsChange{}, nil)

	// Setup for service call after notify event.
	changeInfo := relation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"joni/1": 23,
		},
	}
	s.service.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(changeInfo, nil)

	// Setup the wrapped watcher for test.
	w, err := wrappedRelationChangesWatcher(
		source,
		appUUID,
		relUUID,
		s.service,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer w.Kill()

	// Act
	s.send(c, source.changes)

	// Stop without accepting the output event.
	stopped := make(chan error)
	go func() {
		err := worker.Stop(w)
		stopped <- err
	}()

	// Assert
	select {
	case err := <-stopped:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for watcher to stop with pending send")
	}
}

func (s *relationUnitsWatcherSuite) send(c *tc.C, ch chan struct{}) {
	select {
	case ch <- struct{}{}:
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
	changes chan struct{}
}

func (w *mockRUWatcher) Changes() <-chan struct{} {
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
