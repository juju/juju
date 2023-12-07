// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewaller"
	"github.com/juju/juju/worker/caasfirewaller/mocks"
)

type workerSuite struct {
	testing.BaseSuite

	config caasfirewaller.Config

	firewallerAPI *mocks.MockCAASFirewallerAPI
	lifeGetter    *mocks.MockLifeGetter
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
	s.broker = nil
	s.config = caasfirewaller.Config{}

	s.BaseSuite.TearDownTest(c)
}

func (s *workerSuite) initConfig(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appsWatcher = watchertest.NewMockStringsWatcher(s.applicationChanges)
	s.firewallerAPI = mocks.NewMockCAASFirewallerAPI(ctrl)
	s.firewallerAPI.EXPECT().WatchApplications().AnyTimes().Return(s.appsWatcher, nil)

	s.lifeGetter = mocks.NewMockLifeGetter(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)

	s.config = caasfirewaller.Config{
		ControllerUUID: testing.ControllerTag.Id(),
		ModelUUID:      testing.ModelTag.Id(),
		FirewallerAPI:  s.firewallerAPI,
		Broker:         s.broker,
		LifeGetter:     s.lifeGetter,
		Logger:         loggo.GetLogger("test"),
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.initConfig(c)

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

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*caasfirewaller.Config), expect string) {
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
		firewallerAPI caasfirewaller.CAASFirewallerAPI,
		broker caasfirewaller.CAASBroker,
		lifeGetter caasfirewaller.LifeGetter,
		logger caasfirewaller.Logger,
	) (worker.Worker, error) {
		if appName == "app1" {
			return app1Worker, nil
		} else if appName == "app2" {
			return app2Worker, nil
		}
		return nil, errors.New("never happen")
	}

	charmInfo := &charms.CharmInfo{
		Meta:     &charm.Meta{},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}}, // bases make it a v2 charm
	}
	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app1").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life("app1").Return(life.Alive, nil)
	// Added app1's worker to catacomb.
	app1Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app2").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life("app2").Return(life.Alive, nil)
	// Added app2's worker to catacomb.
	app2Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app1").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life("app1").Return(life.Value(""), errors.NotFoundf("%q", "app1"))
	// Stopped app1's worker because it's removed.
	app1Worker.EXPECT().Kill()
	app1Worker.EXPECT().Wait().Return(nil)

	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app2").Return(charmInfo, nil)
	s.lifeGetter.EXPECT().Life("app2").Return(life.Dead, nil)
	// Stopped app2's worker because it's dead.
	app2Worker.EXPECT().Kill()
	app2Worker.EXPECT().Wait().Return(nil)

	w, err := caasfirewaller.NewWorkerForTest(s.config, workerCreator)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestV1CharmSkipsProcessing(c *gc.C) {
	ctrl := s.initConfig(c)
	defer ctrl.Finish()

	go func() {
		s.applicationChanges <- []string{"app1"}
	}()

	charmInfo := &charms.CharmInfo{ // v1 charm
		Meta:     &charm.Meta{},
		Manifest: &charm.Manifest{},
	}
	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app1").Return(charmInfo, nil)

	w, err := caasfirewaller.NewWorkerForTest(s.config, nil)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotFoundCharmSkipsProcessing(c *gc.C) {
	ctrl := s.initConfig(c)
	defer ctrl.Finish()

	go func() {
		s.applicationChanges <- []string{"app1"}
	}()

	s.firewallerAPI.EXPECT().ApplicationCharmInfo("app1").Return(nil, errors.NotFoundf("app1"))

	w, err := caasfirewaller.NewWorkerForTest(s.config, nil)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}
