// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/uuid"
)

type modelRemovalSuite struct {
	modelService *MockModelService
	modelUUID    coremodel.UUID
}

func TestModelRemovalSuite(t *testing.T) {
	tc.Run(t, &modelRemovalSuite{})
}

func (s *modelRemovalSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockModelService(ctrl)
	s.modelUUID = coremodel.UUID(uuid.MustNewUUID().String())
	return ctrl
}

// fakeServedConnection implements servedConnection, recording closure and
// letting tests drive the dead channel.
type fakeServedConnection struct {
	dead      chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeServedConnection() *fakeServedConnection {
	return &fakeServedConnection{
		dead:   make(chan struct{}),
		closed: make(chan struct{}),
	}
}

func (f *fakeServedConnection) Dead() <-chan struct{} {
	return f.dead
}

func (f *fakeServedConnection) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeServedConnection) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}

// stubModelRemovalWatchService returns a fixed watcher or error from
// WatchModel.
type stubModelRemovalWatchService struct {
	watch watcher.NotifyWatcher
	err   error
}

func (s stubModelRemovalWatchService) WatchModel(context.Context, coremodel.UUID) (watcher.NotifyWatcher, error) {
	return s.watch, s.err
}

// runWatch drives runModelRemovalWatch in a goroutine and returns a channel
// that is closed when it returns.
func runWatch(srv *Server, ctx context.Context, conn servedConnection, modelService ModelService, watch watcher.NotifyWatcher, modelUUID coremodel.UUID) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.runModelRemovalWatch(ctx, conn, modelService, watch, modelUUID)
	}()
	return done
}

// TestWatchNotStartedWhenWatcherCreationFails pins the best-effort contract:
// if the removal watcher cannot be created, the connection is served without
// it, exactly as connections always have been.
func (s *modelRemovalSuite) TestWatchNotStartedWhenWatcherCreationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	conn := newFakeServedConnection()
	srv := &Server{}

	// The model service must not be consulted when there is no watcher.
	srv.watchServedModelRemoval(c.Context(), conn, s.modelService,
		stubModelRemovalWatchService{err: errors.New("boom")}, s.modelUUID)

	c.Assert(conn.isClosed(), tc.IsFalse)
}

// TestNoRemovalEventLeavesConnectionOpen verifies that a quiet watcher never
// closes the connection, and that cancelling the connection's context stops
// the watch goroutine.
func (s *modelRemovalSuite) TestNoRemovalEventLeavesConnectionOpen(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctx, cancel := context.WithCancel(c.Context())
	conn := newFakeServedConnection()
	srv := &Server{}

	watch := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	done := runWatch(srv, ctx, conn, s.modelService, watch, s.modelUUID)

	cancel()
	<-done

	c.Assert(conn.isClosed(), tc.IsFalse)
}

// TestDeadConnectionStopsWatch verifies that the watch ends with the
// connection without closing it: nothing may outlive the connection.
func (s *modelRemovalSuite) TestDeadConnectionStopsWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	conn := newFakeServedConnection()
	srv := &Server{}

	watch := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	done := runWatch(srv, c.Context(), conn, s.modelService, watch, s.modelUUID)

	close(conn.dead)
	<-done

	c.Assert(conn.isClosed(), tc.IsFalse)
}

// TestModelRemovalClosesConnection is the migration REAP case: the model row
// disappears from the controller, the watcher fires, the model lookup reports
// not found, and the connection is closed so agents reconnect to the target
// controller.
func (s *modelRemovalSuite) TestModelRemovalClosesConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelConnectionInfo(gomock.Any(), s.modelUUID).
		Return(model.ModelConnectionInfo{}, modelerrors.NotFound)

	conn := newFakeServedConnection()
	srv := &Server{}

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	watch := watchertest.NewMockNotifyWatcher(changes)

	srv.watchServedModelRemoval(c.Context(), conn, s.modelService,
		stubModelRemovalWatchService{watch: watch}, s.modelUUID)

	<-conn.closed
}

// TestUnrelatedModelChangeKeepsWatching verifies that a model change which is
// not a removal - the model is still connectable - does not close the
// connection, and that a later removal still does.
func (s *modelRemovalSuite) TestUnrelatedModelChangeKeepsWatching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	firstCheck := make(chan struct{})
	gomock.InOrder(
		s.modelService.EXPECT().GetModelConnectionInfo(gomock.Any(), s.modelUUID).
			DoAndReturn(func(context.Context, coremodel.UUID) (model.ModelConnectionInfo, error) {
				close(firstCheck)
				return model.ModelConnectionInfo{Activated: true}, nil
			}),
		s.modelService.EXPECT().GetModelConnectionInfo(gomock.Any(), s.modelUUID).
			Return(model.ModelConnectionInfo{}, modelerrors.NotFound),
	)

	conn := newFakeServedConnection()
	srv := &Server{}

	changes := make(chan struct{})
	watch := watchertest.NewMockNotifyWatcher(changes)
	done := runWatch(srv, c.Context(), conn, s.modelService, watch, s.modelUUID)

	changes <- struct{}{}
	<-firstCheck
	c.Assert(conn.isClosed(), tc.IsFalse)

	changes <- struct{}{}
	<-conn.closed
	<-done
}

// TestUnactivatedModelClosesConnection covers destroy-model: the
// model row may still exist but is no longer connectable. The
// connection must still be closed so agents do not linger.
func (s *modelRemovalSuite) TestUnactivatedModelClosesConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelConnectionInfo(gomock.Any(), s.modelUUID).
		Return(model.ModelConnectionInfo{Activated: false}, nil)

	conn := newFakeServedConnection()
	srv := &Server{}

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	watch := watchertest.NewMockNotifyWatcher(changes)
	done := runWatch(srv, c.Context(), conn, s.modelService, watch, s.modelUUID)

	<-conn.closed
	<-done
}
