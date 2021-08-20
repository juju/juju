// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/docker"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasmodelconfigmanager"
	"github.com/juju/juju/worker/caasmodelconfigmanager/mocks"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.IsolationSuite

	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
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
	c.Check(cfg.Validate(), jc.ErrorIsNil)
}

func (s *workerSuite) TestWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockFacade(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)

	controllerConfigChangeChan := make(chan struct{}, 1)
	controllerConfigChangeWatcher := watchertest.NewMockNotifyWatcher(controllerConfigChangeChan)

	controllerConfig := coretesting.FakeControllerConfig()
	controllerConfig[controller.CAASImageRepo] = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}`[1:]

	controllerConfig2 := coretesting.FakeControllerConfig()
	controllerConfig2[controller.CAASImageRepo] = `
{
    "serveraddress": "gcr.io",
    "auth": "yyyy==",
    "repository": "test-account"
}`[1:]

	go func() {
		controllerConfigChangeChan <- struct{}{}
		controllerConfigChangeChan <- struct{}{}
		controllerConfigChangeChan <- struct{}{}
	}()

	done := make(chan struct{}, 1)
	gomock.InOrder(
		facade.EXPECT().WatchControllerConfig().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			return controllerConfigChangeWatcher, nil
		}),
		// 1st round.
		facade.EXPECT().ControllerConfig().Return(controllerConfig, nil),
		broker.EXPECT().EnsureImageRepoSecret(controllerConfig.CAASImageRepo()).DoAndReturn(func(docker.ImageRepoDetails) error {
			return nil
		}),

		// 2nd round - no change, no ops.
		facade.EXPECT().ControllerConfig().DoAndReturn(func() (controller.Config, error) {
			return controllerConfig, nil
		}),
		// 3rd round - got config changes, apply now.
		facade.EXPECT().ControllerConfig().Return(controllerConfig2, nil),
		broker.EXPECT().EnsureImageRepoSecret(controllerConfig2.CAASImageRepo()).DoAndReturn(func(docker.ImageRepoDetails) error {
			close(done)
			return nil
		}),
	)

	cfg := caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Logger:   s.logger,
		Facade:   facade,
		Broker:   broker,
	}
	w, err := caasmodelconfigmanager.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, w)
	})

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}
}
