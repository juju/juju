// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"encoding/base64"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	registrymocks "github.com/juju/juju/docker/registry/mocks"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasmodelconfigmanager"
	"github.com/juju/juju/worker/caasmodelconfigmanager/mocks"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.IsolationSuite

	modelTag names.ModelTag
	logger   loggo.Logger

	facade           *mocks.MockFacade
	broker           *mocks.MockCAASBroker
	reg              *registrymocks.MockRegistry
	clock            *testclock.Clock
	controllerConfig controller.Config
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
	s.controllerConfig = coretesting.FakeControllerConfig()
	s.clock = testclock.NewClock(time.Time{})
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.facade = nil
}

func (s *workerSuite) TestConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := caasmodelconfigmanager.Config{}
	c.Check(cfg.Validate(), gc.ErrorMatches, `ModelTag is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Facade is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Broker is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Logger is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
		Logger:   s.logger,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Clock is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
		Logger:   s.logger,
		Clock:    s.clock,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `RegistryFunc is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:     s.modelTag,
		Facade:       mocks.NewMockFacade(ctrl),
		Broker:       mocks.NewMockCAASBroker(ctrl),
		Logger:       s.logger,
		Clock:        s.clock,
		RegistryFunc: func(i docker.ImageRepoDetails) (registry.Registry, error) { return nil, nil },
	}
	c.Check(cfg.Validate(), jc.ErrorIsNil)
}

func (s *workerSuite) getWorkerStarter(c *gc.C) (func(...*gomock.Call) worker.Worker, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacade(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.reg = registrymocks.NewMockRegistry(ctrl)

	cfg := caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Logger:   s.logger,
		Facade:   s.facade,
		Broker:   s.broker,
		Clock:    s.clock,
		RegistryFunc: func(i docker.ImageRepoDetails) (registry.Registry, error) {
			c.Assert(i, gc.DeepEquals, s.controllerConfig.CAASImageRepo())
			return s.reg, nil
		},
	}
	return func(calls ...*gomock.Call) worker.Worker {
		gomock.InOrder(calls...)
		w, err := caasmodelconfigmanager.NewWorker(cfg)
		c.Assert(err, jc.ErrorIsNil)
		s.AddCleanup(func(c *gc.C) {
			workertest.CleanKill(c, w)
		})
		return w
	}, ctrl
}

func (s *workerSuite) TestWorkerTokenRefreshRequired(c *gc.C) {
	s.controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "username": "aws_access_key_id",
    "repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "password": "aws_secret_access_key",
    "region": "ap-southeast-2"
}`[1:]

	refreshed := s.controllerConfig.CAASImageRepo()
	refreshed.Auth = docker.NewToken(`refreshed===`)

	done := make(chan struct{}, 1)
	startWorker, ctrl := s.getWorkerStarter(c)
	defer ctrl.Finish()

	_ = startWorker(
		// 1st round.
		s.facade.EXPECT().ControllerConfig().Return(s.controllerConfig, nil),
		s.reg.EXPECT().Ping().Return(nil),
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, nil),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().DoAndReturn(
			func() docker.ImageRepoDetails {
				o := s.controllerConfig.CAASImageRepo()
				c.Assert(o, gc.DeepEquals, docker.ImageRepoDetails{
					ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
					Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
					Region:        "ap-southeast-2",
					BasicAuthConfig: docker.BasicAuthConfig{
						Username: "aws_access_key_id",
						Password: "aws_secret_access_key",
						Auth:     docker.NewToken(base64.StdEncoding.EncodeToString([]byte("aws_access_key_id:aws_secret_access_key"))),
					},
				})
				return o
			},
		),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any()).DoAndReturn(
			func(i docker.ImageRepoDetails) error {
				c.Assert(i, gc.DeepEquals, s.controllerConfig.CAASImageRepo())
				return nil
			},
		),
		// 2nd round.
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, nil),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().Return(refreshed),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any()).DoAndReturn(
			func(i docker.ImageRepoDetails) error {
				c.Assert(i, gc.DeepEquals, refreshed)
				close(done)
				return nil
			},
		),
	)

	err := s.clock.WaitAdvance(30*time.Second, coretesting.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}
}

func (s *workerSuite) TestWorkerTokenRefreshNotRequiredThenRetry(c *gc.C) {
	s.controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "username": "aws_access_key_id",
    "repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "password": "aws_secret_access_key",
    "region": "ap-southeast-2"
}`[1:]

	done := make(chan struct{}, 1)
	startWorker, ctrl := s.getWorkerStarter(c)
	defer ctrl.Finish()

	_ = startWorker(
		// 1st round.
		s.facade.EXPECT().ControllerConfig().Return(s.controllerConfig, nil),
		s.reg.EXPECT().Ping().Return(nil),
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, nil),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().DoAndReturn(
			func() docker.ImageRepoDetails {
				o := s.controllerConfig.CAASImageRepo()
				c.Assert(o, gc.DeepEquals, docker.ImageRepoDetails{
					ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
					Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
					Region:        "ap-southeast-2",
					BasicAuthConfig: docker.BasicAuthConfig{
						Username: "aws_access_key_id",
						Password: "aws_secret_access_key",
						Auth:     docker.NewToken(base64.StdEncoding.EncodeToString([]byte("aws_access_key_id:aws_secret_access_key"))),
					},
				})
				return o
			},
		),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any()).DoAndReturn(
			func(i docker.ImageRepoDetails) error {
				c.Assert(i, gc.DeepEquals, s.controllerConfig.CAASImageRepo())
				return nil
			},
		),
		// 2nd round.
		s.reg.EXPECT().ShouldRefreshAuth().DoAndReturn(func() (bool, *time.Duration) {
			nextTick := 7 * time.Minute
			return false, &nextTick
		}),
		// 3rd round.
		s.reg.EXPECT().ShouldRefreshAuth().DoAndReturn(func() (bool, *time.Duration) {
			return true, nil
		}),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().Return(s.controllerConfig.CAASImageRepo()),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any()).DoAndReturn(
			func(i docker.ImageRepoDetails) error {
				c.Assert(i, gc.DeepEquals, s.controllerConfig.CAASImageRepo())
				close(done)
				return nil
			},
		),
	)

	err := s.clock.WaitAdvance(30*time.Second, coretesting.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.clock.WaitAdvance(7*time.Minute, coretesting.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}
}

func (s *workerSuite) TestWorkerNoOpsForPublicRepo(c *gc.C) {
	s.controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "region": "ap-southeast-2",
}`[1:]

	done := make(chan struct{}, 1)
	startWorker, ctrl := s.getWorkerStarter(c)
	defer ctrl.Finish()

	_ = startWorker(
		s.facade.EXPECT().ControllerConfig().DoAndReturn(func() (controller.Config, error) {
			close(done)
			return s.controllerConfig, nil
		}),
	)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}
}
