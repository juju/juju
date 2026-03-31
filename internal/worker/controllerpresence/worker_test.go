// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerpresence

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/api"
	coreerrors "github.com/juju/juju/core/errors"
	machine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	apiremotecaller "github.com/juju/juju/internal/worker/apiremotecaller"
)

func TestWorker(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &WorkerSuite{})
}

type WorkerSuite struct {
	testhelpers.IsolationSuite

	statusService       *MockStatusService
	apiRemoteSubscriber *MockAPIRemoteSubscriber
	subscription        *MockSubscription
	connection          *MockConnection
	remoteConnection    *MockRemoteConnection
}

func (s *WorkerSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.newConfig(c).Validate()
	c.Assert(err, tc.IsNil)

	config := s.newConfig(c)
	config.StatusService = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.APIRemoteSubscriber = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.Logger = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.Clock = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *WorkerSuite) TestWorkerNoInitialRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.apiRemoteSubscriber.EXPECT().Subscribe().Return(s.subscription, nil)
	s.apiRemoteSubscriber.EXPECT().GetAPIRemotes().DoAndReturn(func() ([]apiremotecaller.RemoteConnection, error) {
		return nil, nil
	})
	s.subscription.EXPECT().Changes().DoAndReturn(func() <-chan struct{} {
		close(done)
		return nil
	})
	s.subscription.EXPECT().Close()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("worker did not start")
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerInitialRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Wait for the worker to create the initial connection.

	done := make(chan struct{})
	wait := make(chan struct{})
	s.apiRemoteSubscriber.EXPECT().Subscribe().Return(s.subscription, nil)
	s.apiRemoteSubscriber.EXPECT().GetAPIRemotes().DoAndReturn(func() ([]apiremotecaller.RemoteConnection, error) {
		return []apiremotecaller.RemoteConnection{remoteConnection{
			controllerID: "0",
			fn: func() error {
				close(done)
				<-wait
				return nil
			},
		}}, nil
	})
	s.subscription.EXPECT().Changes().Return(make(<-chan struct{}))
	s.subscription.EXPECT().Close()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("worker did not start")
	}

	c.Assert(w.runner.WorkerNames(), tc.DeepEquals, []string{"controller-0"})

	close(wait)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerRemotesSubscription(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Wait for the worker to create the initial connection.

	first := make(chan struct{})
	done := make(chan struct{})
	ch := make(chan struct{})

	wait0 := make(chan struct{})
	wait1 := make(chan struct{})

	s.apiRemoteSubscriber.EXPECT().Subscribe().Return(s.subscription, nil)
	s.subscription.EXPECT().Close()

	gomock.InOrder(
		s.apiRemoteSubscriber.EXPECT().GetAPIRemotes().DoAndReturn(func() ([]apiremotecaller.RemoteConnection, error) {
			return []apiremotecaller.RemoteConnection{remoteConnection{
				controllerID: "0",
				fn: func() error {
					close(first)
					<-wait0
					return nil
				},
			}}, nil
		}),
		s.subscription.EXPECT().Changes().Return(ch),
		s.apiRemoteSubscriber.EXPECT().GetAPIRemotes().DoAndReturn(func() ([]apiremotecaller.RemoteConnection, error) {
			return []apiremotecaller.RemoteConnection{remoteConnection{
				controllerID: "1",
				fn: func() error {
					close(done)
					<-wait1
					return nil
				},
			}}, nil
		}),
		s.subscription.EXPECT().Changes().Return(ch),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-first:
	case <-c.Context().Done():
		c.Fatal("worker did not start")
	}

	c.Assert(w.runner.WorkerNames(), tc.DeepEquals, []string{"controller-0"})

	close(wait0)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatal("could not send change")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("worker did not start")
	}

	c.Assert(w.runner.WorkerNames(), tc.DeepEquals, []string{"controller-1"})

	close(wait1)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestNewConnectionTrackerBroken(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(ctx context.Context, c api.Connection) error) error {
		return f(ctx, s.connection)
	})
	s.connection.EXPECT().IsBroken(gomock.Any()).Return(false)

	ch := make(chan struct{})
	sync0 := make(chan struct{})
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		defer close(sync0)
		return ch
	})

	s.statusService.EXPECT().DeleteMachinePresence(gomock.Any(), machine.Name("0")).Return(nil)

	sync1 := make(chan struct{})
	s.statusService.EXPECT().DeleteUnitPresence(gomock.Any(), tc.Must1_1(c, unit.NewName, "controller/0")).DoAndReturn(func(ctx context.Context, n unit.Name) error {
		defer close(sync1)
		return nil
	})

	w := s.newConnectionTracker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync0:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not start")
	}

	c.Assert(w.connected.Load(), tc.IsTrue)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not start")
	}

	select {
	case <-sync1:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not handle broken connection")
	}

	c.Assert(w.connected.Load(), tc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestNewConnectionTrackerBrokenImmediately(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(ctx context.Context, c api.Connection) error) error {
		return f(ctx, s.connection)
	})
	s.connection.EXPECT().IsBroken(gomock.Any()).Return(false)

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	sync0 := make(chan struct{})
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		defer close(sync0)
		return ch
	})

	s.statusService.EXPECT().DeleteMachinePresence(gomock.Any(), machine.Name("0")).Return(nil)

	sync1 := make(chan struct{})
	s.statusService.EXPECT().DeleteUnitPresence(gomock.Any(), tc.Must1_1(c, unit.NewName, "controller/0")).DoAndReturn(func(ctx context.Context, n unit.Name) error {
		defer close(sync1)
		return nil
	})

	w := s.newConnectionTracker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync0:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not start")
	}
	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not recv first close notification")
	}

	c.Assert(w.connected.Load(), tc.IsFalse)

	select {
	case <-sync1:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not handle broken connection")
	}

	c.Assert(w.connected.Load(), tc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestNewConnectionTrackerAlreadyBroken(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(ctx context.Context, c api.Connection) error) error {
		return f(ctx, s.connection)
	})
	sync := make(chan struct{})
	s.connection.EXPECT().IsBroken(gomock.Any()).DoAndReturn(func(ctx context.Context) bool {
		defer close(sync)
		return true
	})

	w := s.newConnectionTracker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not start")
	}

	err := workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIs, BrokenConnection)

	c.Assert(w.connected.Load(), tc.IsFalse)
}

func (s *WorkerSuite) TestNewConnectionTrackerReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(ctx context.Context, c api.Connection) error) error {
		return f(ctx, s.connection)
	})
	sync := make(chan struct{})
	s.connection.EXPECT().IsBroken(gomock.Any()).DoAndReturn(func(ctx context.Context) bool {
		defer close(sync)
		return true
	})

	w := s.newConnectionTracker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("connection tracker did not start")
	}

	err := workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIs, BrokenConnection)

	report := w.Report(context.Background())
	c.Assert(report["controller-id"], tc.Equals, "0")
	c.Assert(report["connected"], tc.Equals, false)
}

func (s *WorkerSuite) newConfig(c *tc.C) WorkerConfig {
	return WorkerConfig{
		StatusService:       s.statusService,
		APIRemoteSubscriber: s.apiRemoteSubscriber,
		Clock:               clock.WallClock,
		Logger:              loggertesting.WrapCheckLog(c),
	}
}

func (s *WorkerSuite) newWorker(c *tc.C) *controllerWorker {
	worker, err := newWorker(s.newConfig(c))
	c.Assert(err, tc.IsNil)
	return worker.(*controllerWorker)
}

func (s *WorkerSuite) newConnectionTracker(c *tc.C) *connectionTracker {
	worker, err := newConnectionTracker("0", s.remoteConnection, s.statusService, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.IsNil)
	return worker.(*connectionTracker)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	mockCtrl := gomock.NewController(c)
	s.statusService = NewMockStatusService(mockCtrl)
	s.apiRemoteSubscriber = NewMockAPIRemoteSubscriber(mockCtrl)
	s.subscription = NewMockSubscription(mockCtrl)
	s.connection = NewMockConnection(mockCtrl)
	s.remoteConnection = NewMockRemoteConnection(mockCtrl)

	c.Cleanup(func() {
		s.statusService = nil
		s.apiRemoteSubscriber = nil
		s.subscription = nil
		s.connection = nil
		s.remoteConnection = nil
	})

	return mockCtrl
}

type remoteConnection struct {
	controllerID string
	fn           func() error
}

func (r remoteConnection) ControllerID() string {
	return r.controllerID
}

func (r remoteConnection) Connection(ctx context.Context, fn func(ctx context.Context, c api.Connection) error) error {
	return r.fn()
}
