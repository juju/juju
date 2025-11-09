// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/charms"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type workerSuite struct {
	config FirewallerConfig

	appFirewallerWorker *mocks.MockWorker
	applicationService  *mocks.MockApplicationService
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
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
// implementation of [ApplicationWorkerCreator].
func (s *workerSuite) appFirewallerWorkerCreator(
	coreapplication.UUID,
) (worker.Worker, error) {
	return s.appFirewallerWorker, nil
}

// getValidConfig returns a valid [Config] that can be used for testing in this
// suite.
func (s *workerSuite) getValidConfig(t *stdtesting.T) FirewallerConfig {
	return FirewallerConfig{
		ApplicationService: s.applicationService,
		Logger:             loggertesting.WrapCheckLog(t),
		WorkerCreator:      s.appFirewallerWorkerCreator,
	}
}

// TestValidateConfig ensures that [FirewallerConfig] both passes and fails
// validation for various configurations.
func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Run("valid", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		tc.Check(c, config.Validate(), tc.ErrorIsNil)
	})

	c.Run("nil ApplicationService", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		config.ApplicationService = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing logger", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		config.Logger = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("nil WorkerCreator", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		config.WorkerCreator = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

// TestStartStopAppWorkerOnLifeNotFoundError tests that the application
// firewaller workers are started and stopped correctly when the firewaller
// worker receives a [applicationerrors.ApplicationNotFound] error.
func (s *workerSuite) TestStartStopAppWorkerOnLifeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)
	app2UUID := tc.Must(c, coreapplication.NewUUID)

	appWatcherChan := make(chan []string, 2)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)
	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String(), app2UUID.String()}
	// Trigger to stop app workers. Change order to make sure we are not testing
	// on implementation order.
	appWatcherChan <- []string{app2UUID.String(), app1UUID.String()}

	charmInfo := &charms.CharmInfo{
		Meta:     &internalcharm.Meta{},
		Manifest: &internalcharm.Manifest{Bases: []internalcharm.Base{{}}}, // bases make it a v2 charm
	}

	appSvcExp := s.applicationService.EXPECT()
	// Return v2 charm info for all apps any time. Not the focus of this test.
	appSvcExp.GetCharmByApplicationUUID(gomock.Any(), gomock.Any()).Return(
		charmInfo.Charm(), charm.CharmLocator{}, nil,
	).AnyTimes()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			return appWatcher, nil
		},
	).AnyTimes()

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

	// doneChan is used to signal the occurance of the last event this test
	// wants to witness.
	doneCh := make(chan struct{})
	workerExp := s.appFirewallerWorker.EXPECT()
	workerExp.Wait().Return(nil).Times(3)
	workerExp.Kill().Times(2)
	workerExp.Wait().DoAndReturn(func() error {
		close(doneCh)
		return nil
	})

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	<-doneCh
	w.Kill()
	c.Check(w.Wait(), tc.ErrorIsNil)
}

// TestStartStopAppWorkerOnLifeDead tests that the application firewaller
// workers are started and stopped correctly when the firewaller worker is
// informed the application is dead.
func (s *workerSuite) TestStartStopAppWorkerOnLifeDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)
	app2UUID := tc.Must(c, coreapplication.NewUUID)

	appWatcherChan := make(chan []string, 2)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)
	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String(), app2UUID.String()}
	// Trigger to stop app workers. Change order to make sure we are not testing
	// on implementation order.
	appWatcherChan <- []string{app2UUID.String(), app1UUID.String()}

	charmInfo := &charms.CharmInfo{
		Meta: &internalcharm.Meta{},
		// bases make it a v2 charm
		Manifest: &internalcharm.Manifest{Bases: []internalcharm.Base{{}}},
	}

	appSvcExp := s.applicationService.EXPECT()
	// Return v2 charm info for all apps any time. Not the focus of this test.
	appSvcExp.GetCharmByApplicationUUID(gomock.Any(), gomock.Any()).Return(
		charmInfo.Charm(), charm.CharmLocator{}, nil,
	).AnyTimes()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			return appWatcher, nil
		},
	).AnyTimes()

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

	// doneChan is used to signal the occurance of the last event this test
	// wants to witness.
	doneCh := make(chan struct{})
	workerExp := s.appFirewallerWorker.EXPECT()
	workerExp.Wait().Return(nil).Times(3)
	workerExp.Kill().Times(2)
	workerExp.Wait().DoAndReturn(func() error {
		close(doneCh)
		return nil
	})

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	<-doneCh
	w.Kill()
	c.Check(w.Wait(), tc.ErrorIsNil)
}

// TestStartStopAppWorkerOnCharmFormatNotFound tests that the application
// firewaller does not attempt to start a worker for an application that is
// reported to be not found when inspecting the charms format version.
func (s *workerSuite) TestStartStopAppWorkerOnCharmFormatNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	// doneChan is used to signal the occurance of the last event this test
	// wants to witness.
	doneCh := make(chan struct{})

	appWatcherChan := make(chan []string, 1)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)
	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String()}

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			close(doneCh)
			return appWatcher, nil
		},
	).AnyTimes()
	appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)
	appSvcExp.GetCharmByApplicationUUID(gomock.Any(), app1UUID).Return(
		nil, charm.CharmLocator{}, applicationerrors.ApplicationNotFound,
	)

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	<-doneCh
	w.Kill()
	c.Check(w.Wait(), tc.ErrorIsNil)
}

func (s *workerSuite) TestApplicationWithCharmFormatV1NotStarted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	// doneChan is used to signal the occurance of the last event this test
	// wants to witness.
	doneCh := make(chan struct{})

	charmInfo := &charms.CharmInfo{
		Meta:     &internalcharm.Meta{},
		Manifest: &internalcharm.Manifest{Bases: nil},
	}

	appWatcherChan := make(chan []string, 1)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)
	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String()}

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			close(doneCh)
			return appWatcher, nil
		},
	).AnyTimes()
	appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)
	appSvcExp.GetCharmByApplicationUUID(gomock.Any(), app1UUID).Return(
		charmInfo.Charm(), charm.CharmLocator{}, applicationerrors.ApplicationNotFound,
	)

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	<-doneCh
	w.Kill()
	c.Check(w.Wait(), tc.ErrorIsNil)
}

// TestSingleWorkerPerApplication ensures that given multiple watcher events
// for the same application uuid only a single worker is ever started.
func (s *workerSuite) TestSingleWorkerPerApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	// doneChan is used to signal the occurance of the last event this test
	// wants to witness.
	doneCh := make(chan struct{})

	appWatcherChan := make(chan []string, 2)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)
	// Trigger to start app workers.
	appWatcherChan <- []string{app1UUID.String()}
	// Trigger for same application uuid again
	appWatcherChan <- []string{app1UUID.String()}

	charmInfo := &charms.CharmInfo{
		Meta: &internalcharm.Meta{},
		// bases make it a v2 charm
		Manifest: &internalcharm.Manifest{Bases: []internalcharm.Base{{}}},
	}

	appSvcExp := s.applicationService.EXPECT()
	// Return v2 charm info for all apps any time. Not the focus of this test.
	appSvcExp.GetCharmByApplicationUUID(gomock.Any(), gomock.Any()).Return(
		charmInfo.Charm(), charm.CharmLocator{}, nil,
	).AnyTimes()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			return appWatcher, nil
		},
	).AnyTimes()

	// 1st set of events
	appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).Return(
		life.Alive, nil,
	)

	// 2nd set of events
	appSvcExp.GetApplicationLife(gomock.Any(), app1UUID).DoAndReturn(
		func(context.Context, coreapplication.UUID) (life.Value, error) {
			close(doneCh)
			return life.Alive, nil
		},
	)

	workerExp := s.appFirewallerWorker.EXPECT()
	// Make sure application worker is correctly started and shut down with the
	// firewaller worker.
	workerExp.Wait().Return(nil).AnyTimes()
	workerExp.Kill().Times(1)

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	<-doneCh
	w.Kill()
	c.Check(w.Wait(), tc.ErrorIsNil)
}

// TestWatcherChannelCloseStopsWorker ensures that if the application watcher
// channel becomes closed the worker stops in error.
func (s *workerSuite) TestWatcherChannelCloseStopsWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appWatcherChan := make(chan []string)
	appWatcher := watchertest.NewMockStringsWatcher(appWatcherChan)

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.WatchApplications(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.Watcher[[]string], error) {
			return appWatcher, nil
		},
	).AnyTimes()

	w, err := NewFirewallerWorker(s.getValidConfig(c.T))
	c.Assert(err, tc.ErrorIsNil)

	close(appWatcherChan)
	c.Check(w.Wait(), tc.NotNil)
}
