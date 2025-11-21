// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"testing"
	"testing/synctest"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

// firewallerSuite defines a set of tests for asserting the contract of the
// [firewaller] worker.
type firewallerSuite struct {
	appFirewallerWorker *mocks.MockWorker
	applicationService  *mocks.MockApplicationService
}

// TestFirewallerSuite runs the tests defined in [firewallerSuite].
func TestFirewallerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &firewallerSuite{})
}

func (s *firewallerSuite) setupMocks(c *testing.T) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appFirewallerWorker = mocks.NewMockWorker(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)

	c.Cleanup(func() {
		s.appFirewallerWorker = nil
		s.applicationService = nil
	})
	return ctrl
}

// appFirewallerWorkerCreator is a testing helper for this suite to provide an
// implementation of [AppFirewallerWokerCreator].
func (s *firewallerSuite) appFirewallerWorkerCreator(
	coreapplication.UUID,
) (worker.Worker, error) {
	return NewBlockedWorker()
}

// getValidConfig returns a valid [Config] that can be used for testing in this
// suite.
func (s *firewallerSuite) getValidConfig(t *testing.T) FirewallerConfig {
	return FirewallerConfig{
		ApplicationService: s.applicationService,
		Clock:              clock.WallClock,
		Logger:             loggertesting.WrapCheckLog(t),
		WorkerCreator:      s.appFirewallerWorkerCreator,
	}
}

// TestValidateConfig ensures that [FirewallerConfig] both passes and fails
// validation for various configurations.
func (s *firewallerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	c.Run("valid", func(c *testing.T) {
		config := s.getValidConfig(c)
		tc.Check(c, config.Validate(), tc.ErrorIsNil)
	})

	c.Run("nil ApplicationService", func(c *testing.T) {
		config := s.getValidConfig(c)
		config.ApplicationService = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("nil Clock", func(c *testing.T) {
		config := s.getValidConfig(c)
		config.Clock = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing logger", func(c *testing.T) {
		config := s.getValidConfig(c)
		config.Logger = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("nil WorkerCreator", func(c *testing.T) {
		config := s.getValidConfig(c)
		config.WorkerCreator = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

// TestStartStopAppWorkerOnLifeNotFoundError tests that the application
// firewaller workers are started and stopped correctly when the firewaller
// worker receives a [applicationerrors.ApplicationNotFound] error.
func (s *firewallerSuite) TestStartStopAppWorkerOnLifeNotFoundError(c *tc.C) {
	synctest.Test(c.T, func(c *testing.T) {
		defer s.setupMocks(c).Finish()

		app1UUID := tc.Must(c, coreapplication.NewUUID)
		app2UUID := tc.Must(c, coreapplication.NewUUID)

		appWatcherChan := make(chan []string)
		appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

		appSvcExp := s.applicationService.EXPECT()
		appSvcExp.WatchApplications(gomock.Any()).Return(appWatcher, nil).AnyTimes()

		// 1st set of events
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)
		appSvcExp.GetApplicationLife(gomock.Any(), app2UUID).Return(life.Alive, nil)

		// 2nd set of events, both applications become not found.
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(
			"", applicationerrors.ApplicationNotFound,
		)
		appSvcExp.GetApplicationLife(gomock.Any(), app2UUID).Return(
			"", applicationerrors.ApplicationNotFound,
		)

		w, err := NewFirewallerWorker(s.getValidConfig(c))
		tc.Assert(c, err, tc.ErrorIsNil)

		// Trigger to start app workers.
		appWatcherChan <- []string{app1UUID.String(), app2UUID.String()}
		// Trigger to stop app workers. Change order to make sure we are not testing
		// on implementation order.
		appWatcherChan <- []string{app2UUID.String(), app1UUID.String()}

		// Wait for everything to become durably blocked.
		synctest.Wait()
		w.Kill()
		tc.Check(c, w.Wait(), tc.ErrorIsNil)
	})
}

// TestStartStopAppWorkerOnLifeDead tests that the application firewaller
// workers are started and stopped correctly when the firewaller worker is
// informed the application is dead.
func (s *firewallerSuite) TestStartStopAppWorkerOnLifeDead(c *tc.C) {
	synctest.Test(c.T, func(c *testing.T) {
		defer s.setupMocks(c).Finish()

		app1UUID := tc.Must(c, coreapplication.NewUUID)
		app2UUID := tc.Must(c, coreapplication.NewUUID)

		appWatcherChan := make(chan []string)
		appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

		appSvcExp := s.applicationService.EXPECT()
		appSvcExp.WatchApplications(gomock.Any()).Return(appWatcher, nil).AnyTimes()

		// 1st set of events
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)
		appSvcExp.GetApplicationLife(gomock.Any(), app2UUID).Return(life.Alive, nil)

		// 2nd set of events, both applications become not found.
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(
			life.Dead, nil,
		)
		appSvcExp.GetApplicationLife(gomock.Any(), app2UUID).Return(
			life.Dead, nil,
		)

		w, err := NewFirewallerWorker(s.getValidConfig(c))
		tc.Assert(c, err, tc.ErrorIsNil)

		// Trigger to start app workers.
		appWatcherChan <- []string{app1UUID.String(), app2UUID.String()}
		// Trigger to stop app workers. Change order to make sure we are not testing
		// on implementation order.
		appWatcherChan <- []string{app2UUID.String(), app1UUID.String()}

		// Wait for everything to become durably blocked.
		synctest.Wait()
		w.Kill()
		tc.Check(c, w.Wait(), tc.ErrorIsNil)
	})
}

// TestSingleWorkerPerApplication ensures that given multiple watcher events
// for the same application uuid only a single worker is ever started.
func (s *firewallerSuite) TestSingleWorkerPerApplication(c *tc.C) {
	synctest.Test(c.T, func(c *testing.T) {
		defer s.setupMocks(c).Finish()

		app1UUID := tc.Must(c, coreapplication.NewUUID)

		appWatcherChan := make(chan []string)
		appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

		appSvcExp := s.applicationService.EXPECT()
		appSvcExp.WatchApplications(gomock.Any()).Return(appWatcher, nil).AnyTimes()

		// 1st set of events
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(
			life.Alive, nil,
		)

		// 2nd set of events
		appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)

		var workersCreated int
		newAppFirewallerWorker :=
			func(coreapplication.UUID) (worker.Worker, error) {
				workersCreated++
				return NewBlockedWorker()
			}

		workerExp := s.appFirewallerWorker.EXPECT()
		workerExp.Kill().AnyTimes()
		workerExp.Wait().Return(nil).AnyTimes()

		w, err := NewFirewallerWorker(FirewallerConfig{
			ApplicationService: s.applicationService,
			Clock:              clock.WallClock,
			Logger:             loggertesting.WrapCheckLog(c),
			WorkerCreator:      newAppFirewallerWorker,
		})
		tc.Assert(c, err, tc.ErrorIsNil)

		// Trigger to start app workers.
		appWatcherChan <- []string{app1UUID.String()}
		// Trigger for same application uuid again
		appWatcherChan <- []string{app1UUID.String()}

		// Wait for everything to become durably blocked.
		synctest.Wait()
		w.Kill()
		tc.Check(c, w.Wait(), tc.ErrorIsNil)
		tc.Check(c, workersCreated, tc.Equals, 1)
	})
}

// TestWatcherChannelCloseStopsWorker ensures that if the application watcher
// channel becomes closed the worker stops in error.
func (s *firewallerSuite) TestWatcherChannelCloseStopsWorker(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	appWatcherChan := make(chan []string)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.WatchApplications(gomock.Any()).Return(appWatcher, nil).AnyTimes()

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	close(appWatcherChan)
	c.Check(w.Wait(), tc.NotNil)
}

// TestFailedApplicationWorkerStopsFirewaller is a test to ensure that if a
// child application worker of the [firewaller] worker fails then this cascades
// ultimately shutting down the parent worker.
func (s *firewallerSuite) TestFailedApplicationWorkerStopsFirewaller(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	appWatcherChan := make(chan []string)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.WatchApplications(gomock.Any()).Return(appWatcher, nil).AnyTimes()

	// 1st set of events
	appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(
		life.Alive, nil,
	)

	newAppFirewallerWorker :=
		func(coreapplication.UUID) (worker.Worker, error) {
			return NewFailingWorker(internaltesting.ShortWait)
		}

	workerExp := s.appFirewallerWorker.EXPECT()
	workerExp.Kill().AnyTimes()
	workerExp.Wait().Return(nil).AnyTimes()

	w, err := NewFirewallerWorker(FirewallerConfig{
		ApplicationService: s.applicationService,
		Clock:              clock.WallClock,
		Logger:             loggertesting.WrapCheckLog(c),
		WorkerCreator:      newAppFirewallerWorker,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String()}
	c.Check(w.Wait(), tc.ErrorIs, FailingWorkerError)
}
