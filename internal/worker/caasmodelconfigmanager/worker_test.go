// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	registrymocks "github.com/juju/juju/internal/docker/registry/mocks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager/mocks"
)

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

type workerSuite struct {
	testhelpers.IsolationSuite

	modelTag names.ModelTag
	logger   logger.Logger

	facade           *mocks.MockFacade
	broker           *mocks.MockCAASBroker
	reg              *registrymocks.MockRegistry
	clock            testclock.AdvanceableClock
	controllerConfig controller.Config
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggertesting.WrapCheckLog(c)
	s.controllerConfig = coretesting.FakeControllerConfig()
	s.clock = testclock.NewDilatedWallClock(testhelpers.ShortWait)
}

func (s *workerSuite) TearDownTest(c *tc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.facade = nil
}

func (s *workerSuite) TestConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := caasmodelconfigmanager.Config{}
	c.Check(cfg.Validate(), tc.ErrorMatches, `ModelTag is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `Facade is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `Broker is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `Logger is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
		Logger:   s.logger,
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `Clock is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   mocks.NewMockFacade(ctrl),
		Broker:   mocks.NewMockCAASBroker(ctrl),
		Logger:   s.logger,
		Clock:    s.clock,
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `RegistryFunc is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:     s.modelTag,
		Facade:       mocks.NewMockFacade(ctrl),
		Broker:       mocks.NewMockCAASBroker(ctrl),
		Logger:       s.logger,
		Clock:        s.clock,
		RegistryFunc: func(i docker.ImageRepoDetails) (registry.Registry, error) { return nil, nil },
	}
	c.Check(cfg.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) getWorkerStarter(c *tc.C) (func(...any) worker.Worker, *gomock.Controller) {
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
			c.Check(i, tc.DeepEquals, s.CAASImageRepo(c))
			return s.reg, nil
		},
	}
	return func(calls ...any) worker.Worker {
		gomock.InOrder(calls...)
		w, err := caasmodelconfigmanager.NewWorker(cfg)
		c.Assert(err, tc.ErrorIsNil)
		return w
	}, ctrl
}

func (s *workerSuite) TestWorkerTokenRefreshRequired(c *tc.C) {
	s.controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "username": "aws_access_key_id",
    "repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "password": "aws_secret_access_key",
    "region": "ap-southeast-2"
}`[1:]

	refreshed := s.CAASImageRepo(c)
	refreshed.Auth = docker.NewToken(`refreshed===`)

	done := make(chan struct{}, 1)
	startWorker, ctrl := s.getWorkerStarter(c)
	defer ctrl.Finish()

	controllerConfigChangedChan := make(chan []string, 1)
	w := startWorker(
		s.facade.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
			controllerConfigChangedChan <- []string{controller.CAASImageRepo}
			return watchertest.NewMockStringsWatcher(controllerConfigChangedChan), nil
		}),
		// 1st round.
		s.facade.EXPECT().ControllerConfig(gomock.Any()).Return(s.controllerConfig, nil),
		s.reg.EXPECT().Ping().Return(nil),
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, time.Duration(0)),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().DoAndReturn(func() docker.ImageRepoDetails {
			o := s.CAASImageRepo(c)
			c.Check(o, tc.DeepEquals, docker.ImageRepoDetails{
				ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
				Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
				Region:        "ap-southeast-2",
				BasicAuthConfig: docker.BasicAuthConfig{
					Username: "aws_access_key_id",
					Password: "aws_secret_access_key",
				},
			})
			return o
		}),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, i docker.ImageRepoDetails) error {
			c.Check(i, tc.DeepEquals, s.CAASImageRepo(c))
			return nil
		}),
		// 2nd round.
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, time.Duration(0)),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().Return(refreshed),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, i docker.ImageRepoDetails) error {
			c.Check(i, tc.DeepEquals, refreshed)
			close(done)
			return nil
		}),
		s.reg.EXPECT().Close().Return(nil),
	)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerTokenRefreshNotRequiredThenRetry(c *tc.C) {
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

	controllerConfigChangedChan := make(chan []string, 1)
	w := startWorker(
		s.facade.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
			controllerConfigChangedChan <- []string{controller.CAASImageRepo}
			return watchertest.NewMockStringsWatcher(controllerConfigChangedChan), nil
		}),
		// 1st round.
		s.facade.EXPECT().ControllerConfig(gomock.Any()).Return(s.controllerConfig, nil),
		s.reg.EXPECT().Ping().Return(nil),
		s.reg.EXPECT().ShouldRefreshAuth().Return(true, time.Duration(0)),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().DoAndReturn(func() docker.ImageRepoDetails {
			o := s.CAASImageRepo(c)
			c.Check(o, tc.DeepEquals, docker.ImageRepoDetails{
				ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
				Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
				Region:        "ap-southeast-2",
				BasicAuthConfig: docker.BasicAuthConfig{
					Username: "aws_access_key_id",
					Password: "aws_secret_access_key",
				},
			})
			return o
		}),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, i docker.ImageRepoDetails) error {
			c.Check(i, tc.DeepEquals, s.CAASImageRepo(c))
			return nil
		}),
		// 2nd round.
		s.reg.EXPECT().ShouldRefreshAuth().DoAndReturn(func() (bool, time.Duration) {
			return false, 1 * time.Second
		}),
		// 3rd round.
		s.reg.EXPECT().ShouldRefreshAuth().DoAndReturn(func() (bool, time.Duration) {
			return true, time.Duration(0)
		}),
		s.reg.EXPECT().RefreshAuth().Return(nil),
		s.reg.EXPECT().ImageRepoDetails().Return(s.CAASImageRepo(c)),
		s.broker.EXPECT().EnsureImageRepoSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, i docker.ImageRepoDetails) error {
			c.Check(i, tc.DeepEquals, s.CAASImageRepo(c))
			close(done)
			return nil
		}),
		s.reg.EXPECT().Close().Return(nil),
	)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerNoOpsForPublicRepo(c *tc.C) {
	s.controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "region": "ap-southeast-2",
}`[1:]

	done := make(chan struct{}, 1)
	startWorker, ctrl := s.getWorkerStarter(c)
	defer ctrl.Finish()

	controllerConfigChangedChan := make(chan []string, 1)
	w := startWorker(
		s.facade.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
			controllerConfigChangedChan <- []string{controller.CAASImageRepo}
			return watchertest.NewMockStringsWatcher(controllerConfigChangedChan), nil
		}),
		s.facade.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (controller.Config, error) {
			close(done)
			return s.controllerConfig, nil
		}),
	)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) CAASImageRepo(c *tc.C) docker.ImageRepoDetails {
	r, err := docker.NewImageRepoDetails(s.controllerConfig.CAASImageRepo())
	c.Assert(err, tc.ErrorIsNil)
	return r
}
