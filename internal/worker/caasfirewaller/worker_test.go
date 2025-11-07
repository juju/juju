// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/charms"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type workerSuite struct {
	testing.BaseSuite

	config caasfirewaller.Config

	applicationService *mocks.MockApplicationService
	portService        *mocks.MockPortService
	broker             *mocks.MockCAASBroker

	applicationChanges chan []string
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

// getValidConfig returns a valid [Config] that can be used for testing in this
// suite.
func (s *workerSuite) getValidConfig(t *stdtesting.T) caasfirewaller.Config {
	return caasfirewaller.Config{
		ApplicationService: s.applicationService,
		PortService:        s.portService,
		Broker:             s.broker,
		Logger:             loggertesting.WrapCheckLog(t),
	}
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Run("valid", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		tc.Check(c, config.Validate(), tc.ErrorIsNil)
	})

	c.Run("missing broker", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		config.Broker = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing logger", func(c *stdtesting.T) {
		config := s.getValidConfig(c)
		config.Logger = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

func (s *workerSuite) testValidateConfig(c *tc.C, f func(*caasfirewaller.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), tc.ErrorMatches, expect)
}

func (s *workerSuite) TestStartStop(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)
	app2UUID := tc.Must(c, coreapplication.NewUUID)

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		// trigger to start app workers.
		select {
		case s.applicationChanges <- []string{app1UUID.String(), app2UUID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
		// trigger to stop app workers.
		select {
		case s.applicationChanges <- []string{app1UUID.String(), app2UUID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	app1Worker := mocks.NewMockWorker(ctrl)
	app2Worker := mocks.NewMockWorker(ctrl)

	workerCreator := func(
		appUUID coreapplication.UUID,
		portService caasfirewaller.PortService,
		applicationService caasfirewaller.ApplicationService,
		broker caasfirewaller.CAASBroker,
		logger logger.Logger,
	) (worker.Worker, error) {
		if appUUID == app1UUID {
			return app1Worker, nil
		} else if appUUID == app2UUID {
			return app2Worker, nil
		}
		return nil, errors.New("never happen")
	}

	done := make(chan struct{})

	charmInfo := &charms.CharmInfo{
		Meta:     &internalcharm.Meta{},
		Manifest: &internalcharm.Manifest{Bases: []internalcharm.Base{{}}}, // bases make it a v2 charm
	}

	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app1UUID).Return(charmInfo.Charm(), charm.CharmLocator{}, nil)
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), app1UUID).Return(life.Alive, nil)
	// Added app1's worker to catacomb.
	app1Worker.EXPECT().Wait().Return(nil)

	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app2UUID).Return(charmInfo.Charm(), charm.CharmLocator{}, nil)
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), app2UUID).Return(life.Alive, nil)
	// Added app2's worker to catacomb.
	app2Worker.EXPECT().Wait().Return(nil)

	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app1UUID).Return(charmInfo.Charm(), charm.CharmLocator{}, nil)
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), app1UUID).Return(life.Value(""), applicationerrors.ApplicationNotFound)
	// Stopped app1's worker because it's removed.
	app1Worker.EXPECT().Kill()
	app1Worker.EXPECT().Wait().Return(nil)

	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app2UUID).Return(charmInfo.Charm(), charm.CharmLocator{}, nil)
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), app2UUID).Return(life.Dead, nil)
	// Stopped app2's worker because it's dead.
	app2Worker.EXPECT().Kill()
	app2Worker.EXPECT().Wait().DoAndReturn(func() error {
		close(done)

		return nil
	})

	w, err := caasfirewaller.NewWorkerForTest(s.config, workerCreator)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sent:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out sending application changes")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestV1CharmSkipsProcessing(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case s.applicationChanges <- []string{app1UUID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	var done = make(chan struct{})
	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app1UUID).DoAndReturn(
		func(ctx context.Context, id coreapplication.UUID) (internalcharm.Charm, charm.CharmLocator, error) {
			close(done)
			charmInfo := &charms.CharmInfo{ // v1 charm
				Meta:     &internalcharm.Meta{},
				Manifest: &internalcharm.Manifest{},
			}
			return charmInfo.Charm(), charm.CharmLocator{}, nil
		},
	)

	w, err := caasfirewaller.NewWorkerForTest(s.config, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sent:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out sending application changes")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for ApplicationCharmInfo")
	}

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotFoundCharmSkipsProcessing(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app1UUID := tc.Must(c, coreapplication.NewUUID)

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case s.applicationChanges <- []string{app1UUID.String()}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	var done = make(chan struct{})
	s.applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), app1UUID).DoAndReturn(
		func(ctx context.Context, id coreapplication.UUID) (internalcharm.Charm, charm.CharmLocator, error) {
			close(done)
			return nil, charm.CharmLocator{}, coreerrors.NotFound
		},
	)

	w, err := caasfirewaller.NewWorkerForTest(s.config, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sent:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out sending application changes")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for ApplicationCharmInfo")
	}

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationChanges = make(chan []string)

	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.portService = mocks.NewMockPortService(ctrl)

	s.applicationService.EXPECT().WatchApplications(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(s.applicationChanges), nil
	}).AnyTimes()

	s.broker = mocks.NewMockCAASBroker(ctrl)

	s.config = caasfirewaller.Config{
		ApplicationService: s.applicationService,
		PortService:        s.portService,
		Broker:             s.broker,
		Logger:             loggertesting.WrapCheckLog(c),
	}
	return ctrl
}
