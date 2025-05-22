// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/charms"
	coreapplication "github.com/juju/juju/core/application"
	testingapplication "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type workerSuite struct {
	testing.BaseSuite

	config caasfirewaller.Config

	firewallerAPI      *mocks.MockCAASFirewallerAPI
	applicationService *mocks.MockApplicationService
	portService        *mocks.MockPortService
	lifeGetter         *mocks.MockLifeGetter
	broker             *mocks.MockCAASBroker

	applicationChanges chan []string
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ControllerUUID = ""
	}, `missing ControllerUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ModelUUID = ""
	}, `missing ModelUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.FirewallerAPI = nil
	}, `missing FirewallerAPI not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.Broker = nil
	}, `missing Broker not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.Logger = nil
	}, `missing Logger not valid`)
}

func (s *workerSuite) testValidateConfig(c *tc.C, f func(*caasfirewaller.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), tc.ErrorMatches, expect)
}

func (s *workerSuite) TestStartStop(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		// trigger to start app workers.
		select {
		case s.applicationChanges <- []string{"app1", "app2"}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
		// trigger to stop app workers.
		select {
		case s.applicationChanges <- []string{"app1", "app2"}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	app1Worker := mocks.NewMockWorker(ctrl)
	app2Worker := mocks.NewMockWorker(ctrl)

	workerCreator := func(
		controllerUUID string,
		modelUUID string,
		appName string,
		appUUID coreapplication.ID,
		firewallerAPI caasfirewaller.CAASFirewallerAPI,
		portService caasfirewaller.PortService,
		broker caasfirewaller.CAASBroker,
		lifeGetter caasfirewaller.LifeGetter,
		logger logger.Logger,
	) (worker.Worker, error) {
		if appName == "app1" {
			return app1Worker, nil
		} else if appName == "app2" {
			return app2Worker, nil
		}
		return nil, errors.New("never happen")
	}

	done := make(chan struct{})

	app1UUID := testingapplication.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "app1").Return(app1UUID, nil).MinTimes(1)

	app2UUID := testingapplication.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "app2").Return(app2UUID, nil).MinTimes(1)

	charmInfo := &charms.CharmInfo{
		Meta:     &charm.Meta{},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}}, // bases make it a v2 charm
	}
	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app1").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life(gomock.Any(), "app1").Return(life.Alive, nil)
	// Added app1's worker to catacomb.
	app1Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app2").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life(gomock.Any(), "app2").Return(life.Alive, nil)
	// Added app2's worker to catacomb.
	app2Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app1").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life(gomock.Any(), "app1").Return(life.Value(""), errors.NotFoundf("%q", "app1"))
	// Stopped app1's worker because it's removed.
	app1Worker.EXPECT().Kill()
	app1Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app2").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life(gomock.Any(), "app2").Return(life.Dead, nil)
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

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case s.applicationChanges <- []string{"app1"}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	var done = make(chan struct{})
	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app1").DoAndReturn(func(ctx context.Context, s string) (*charms.CharmInfo, error) {
		close(done)
		return &charms.CharmInfo{ // v1 charm
			Meta:     &charm.Meta{},
			Manifest: &charm.Manifest{},
		}, nil
	})

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

	sent := make(chan struct{})
	go func() {
		defer close(sent)

		select {
		case s.applicationChanges <- []string{"app1"}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending application changes")
		}
	}()

	var done = make(chan struct{})
	s.firewallerAPI.EXPECT().ApplicationCharmInfo(gomock.Any(), "app1").DoAndReturn(func(ctx context.Context, s string) (*charms.CharmInfo, error) {
		close(done)
		return nil, errors.NotFoundf("app1")
	})

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

	s.firewallerAPI = mocks.NewMockCAASFirewallerAPI(ctrl)

	s.firewallerAPI.EXPECT().WatchApplications(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(s.applicationChanges), nil
	}).AnyTimes()

	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.portService = mocks.NewMockPortService(ctrl)

	s.lifeGetter = mocks.NewMockLifeGetter(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)

	s.config = caasfirewaller.Config{
		ControllerUUID:     testing.ControllerTag.Id(),
		ModelUUID:          testing.ModelTag.Id(),
		FirewallerAPI:      s.firewallerAPI,
		ApplicationService: s.applicationService,
		PortService:        s.portService,
		Broker:             s.broker,
		LifeGetter:         s.lifeGetter,
		Logger:             loggertesting.WrapCheckLog(c),
	}
	return ctrl
}
