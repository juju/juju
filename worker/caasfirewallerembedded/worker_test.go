// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewallerembedded"
	"github.com/juju/juju/worker/caasfirewallerembedded/mocks"
)

type workerSuite struct {
	testing.BaseSuite

	config caasfirewallerembedded.Config
	w      worker.Worker

	firewallerAPI *mocks.MockCAASFirewallerAPI
	lifeGetter    *mocks.MockLifeGetter
	logger        *mocks.MockLogger
	broker        *mocks.MockCAASBroker

	applicationChanges chan []string

	appsWatcher watcher.StringsWatcher
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	s.applicationChanges = nil

	s.firewallerAPI = nil
	s.lifeGetter = nil
	s.logger = nil
	s.broker = nil
	s.config = caasfirewallerembedded.Config{}

	s.BaseSuite.TearDownTest(c)
}

func (s *workerSuite) initConfig(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appsWatcher = watchertest.NewMockStringsWatcher(s.applicationChanges)
	s.firewallerAPI = mocks.NewMockCAASFirewallerAPI(ctrl)
	s.firewallerAPI.EXPECT().WatchApplications().AnyTimes().Return(s.appsWatcher, nil)

	s.lifeGetter = mocks.NewMockLifeGetter(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)

	s.config = caasfirewallerembedded.Config{
		ControllerUUID: testing.ControllerTag.Id(),
		ModelUUID:      testing.ModelTag.Id(),
		FirewallerAPI:  s.firewallerAPI,
		Broker:         s.broker,
		LifeGetter:     s.lifeGetter,
		Logger:         s.logger,
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.initConfig(c)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.ControllerUUID = ""
	}, `missing ControllerUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.ModelUUID = ""
	}, `missing ModelUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.FirewallerAPI = nil
	}, `missing FirewallerAPI not valid`)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.Broker = nil
	}, `missing Broker not valid`)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasfirewallerembedded.Config) {
		config.Logger = nil
	}, `missing Logger not valid`)
}

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*caasfirewallerembedded.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := s.initConfig(c)
	defer ctrl.Finish()

	go func() {
		// trigger to start app workers.
		s.applicationChanges <- []string{"app1", "app2"}
		// trigger to stop app workers.
		s.applicationChanges <- []string{"app1", "app2"}
	}()

	app1Worker := mocks.NewMockWorker(ctrl)
	app2Worker := mocks.NewMockWorker(ctrl)

	workerCreator := func(
		controllerUUID string,
		modelUUID string,
		appName string,
		firewallerAPI caasfirewallerembedded.CAASFirewallerAPI,
		broker caasfirewallerembedded.CAASBroker,
		lifeGetter caasfirewallerembedded.LifeGetter,
		logger caasfirewallerembedded.Logger,
	) (worker.Worker, error) {
		if appName == "app1" {
			return app1Worker, nil
		} else if appName == "app2" {
			return app2Worker, nil
		}
		return nil, errors.New("never happen")
	}

	s.lifeGetter.EXPECT().Life("app1").Return(life.Alive, nil)
	// Added app1's worker to catacomb.
	app1Worker.EXPECT().Wait().Return(nil)

	s.lifeGetter.EXPECT().Life("app2").Return(life.Alive, nil)
	// Added app2's worker to catacomb.
	app2Worker.EXPECT().Wait().Return(nil)

	s.lifeGetter.EXPECT().Life("app1").Return(life.Value(""), errors.NotFoundf("%q", "app1"))
	// Stopped app1's worker.
	app1Worker.EXPECT().Kill()
	app1Worker.EXPECT().Wait().Return(nil)

	s.lifeGetter.EXPECT().Life("app2").Return(life.Value(""), errors.NotFoundf("%q", "app2"))
	// Stopped app2's worker.
	app2Worker.EXPECT().Kill()
	app2Worker.EXPECT().Wait().Return(nil)

	w, err := caasfirewallerembedded.NewWorkerForTest(s.config, workerCreator)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}
