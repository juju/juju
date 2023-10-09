// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasmodelconfigmanager"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.IsolationSuite

	modelTag names.ModelTag
	logger   loggo.Logger

	controllerConfigService *MockControllerConfigService
	registry                *MockRegistry
	imageRepo               *MockImageRepo
	broker                  *MockCAASBroker

	clock            clock.Clock
	controllerConfig controller.Config
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
	s.controllerConfig = coretesting.FakeControllerConfig()
	s.clock = clock.WallClock
}

func (s *workerSuite) TestConfigValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := caasmodelconfigmanager.Config{}
	c.Check(cfg.Validate(), gc.ErrorMatches, `ModelTag is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `ControllerConfigService is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Broker is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Logger is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
		Logger:                  s.logger,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Clock is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
		Logger:                  s.logger,
		Clock:                   s.clock,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `RegistryFunc is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
		Logger:                  s.logger,
		Clock:                   s.clock,
		RegistryFunc:            func(i docker.ImageRepoDetails) (caasmodelconfigmanager.Registry, error) { return nil, nil },
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `ImageRepoFunc is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
		Logger:                  s.logger,
		Clock:                   s.clock,
		RegistryFunc:            func(i docker.ImageRepoDetails) (caasmodelconfigmanager.Registry, error) { return nil, nil },
		ImageRepoFunc:           func(path string) (caasmodelconfigmanager.ImageRepo, error) { return nil, nil },
	}
	c.Check(cfg.Validate(), jc.ErrorIsNil)
}

func (s *workerSuite) TestCAASImageRepoNotSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfig[controller.CAASImageRepo] = ""
	s.controllerConfigService.EXPECT().ControllerConfig().Return(s.controllerConfig, nil)

	w, err := caasmodelconfigmanager.NewWorker(s.config())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	w.Kill()
	c.Assert(errors.Is(w.Wait(), dependency.ErrUninstall), jc.IsTrue)
}

func (s *workerSuite) TestCAASImageRepoSetButInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfig[controller.CAASImageRepo] = "foo"
	s.controllerConfigService.EXPECT().ControllerConfig().Return(s.controllerConfig, nil)

	s.imageRepo.EXPECT().RequestDetails().Return(docker.ImageRepoDetails{}, nil)

	w, err := caasmodelconfigmanager.NewWorker(s.config())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	w.Kill()
	c.Assert(errors.Is(w.Wait(), dependency.ErrUninstall), jc.IsTrue)
}

func (s *workerSuite) TestCAASImageRepoOnFirstTime(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.expectImageRepoDetails()
	s.expectRegDetails("foo", "bar")
	s.registry.EXPECT().Ping().Return(nil)
	s.registry.EXPECT().ShouldRefreshAuth().Return(false, ptr(time.Millisecond*100))
	s.registry.EXPECT().RefreshAuth().Return(nil)
	s.broker.EXPECT().EnsureImageRepoSecret(imageDetails("foo", "bar")).DoAndReturn(func(docker.ImageRepoDetails) error {
		close(done)
		return nil
	})

	w, err := caasmodelconfigmanager.NewWorker(s.config())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for second request")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestCAASImageRepoWithRefresh(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.expectImageRepoDetails()
	s.expectRegDetails("foo", "bar")
	s.registry.EXPECT().Ping().Return(nil)

	// First request.
	s.registry.EXPECT().ShouldRefreshAuth().Return(true, ptr(time.Millisecond*100))
	s.registry.EXPECT().RefreshAuth().Return(nil)
	s.broker.EXPECT().EnsureImageRepoSecret(imageDetails("foo", "bar")).Return(nil)

	// Second request.
	s.registry.EXPECT().ShouldRefreshAuth().DoAndReturn(func() (bool, *time.Duration) {
		close(done)
		return false, nil
	})

	w, err := caasmodelconfigmanager.NewWorker(s.config())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for second request")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.registry = NewMockRegistry(ctrl)
	s.imageRepo = NewMockImageRepo(ctrl)
	s.broker = NewMockCAASBroker(ctrl)

	return ctrl
}

func (s *workerSuite) config() caasmodelconfigmanager.Config {
	return caasmodelconfigmanager.Config{
		ModelTag:                s.modelTag,
		ControllerConfigService: s.controllerConfigService,
		Broker:                  s.broker,
		Logger:                  s.logger,
		Clock:                   s.clock,
		RegistryFunc: func(i docker.ImageRepoDetails) (caasmodelconfigmanager.Registry, error) {
			return s.registry, nil
		},
		ImageRepoFunc: func(path string) (caasmodelconfigmanager.ImageRepo, error) {
			return s.imageRepo, nil
		},
	}
}

func (s *workerSuite) expectImageRepoDetails() {
	s.controllerConfig[controller.CAASImageRepo] = `
	{
		"serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		"username": "aws_access_key_id",
		"repository": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		"password": "aws_secret_access_key",
		"region": "ap-southeast-2"
	}`
	s.controllerConfigService.EXPECT().ControllerConfig().Return(s.controllerConfig, nil)
	s.imageRepo.EXPECT().RequestDetails().Return(docker.ImageRepoDetails{
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
			Auth:     docker.NewToken(base64.StdEncoding.EncodeToString([]byte("aws_access_key_id:aws_secret_access_key"))),
		},
	}, nil)
}

func (s *workerSuite) expectRegDetails(username, password string) {
	s.registry.EXPECT().ImageRepoDetails().Return(imageDetails(username, password))
}

func imageDetails(username, password string) docker.ImageRepoDetails {
	return docker.ImageRepoDetails{
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: username,
			Password: password,
			Auth:     docker.NewToken(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))),
		},
	}
}

func ptr[T any](t T) *T {
	return &t
}
