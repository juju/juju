// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasmodelconfigmanager"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.IsolationSuite

	modelTag names.ModelTag
	logger   loggo.Logger

	facade           *MockFacade
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
		Facade:   NewMockFacade(ctrl),
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Broker is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   NewMockFacade(ctrl),
		Broker:   NewMockCAASBroker(ctrl),
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Logger is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   NewMockFacade(ctrl),
		Broker:   NewMockCAASBroker(ctrl),
		Logger:   s.logger,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `Clock is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag: s.modelTag,
		Facade:   NewMockFacade(ctrl),
		Broker:   NewMockCAASBroker(ctrl),
		Logger:   s.logger,
		Clock:    s.clock,
	}
	c.Check(cfg.Validate(), gc.ErrorMatches, `RegistryFunc is missing not valid`)

	cfg = caasmodelconfigmanager.Config{
		ModelTag:     s.modelTag,
		Facade:       NewMockFacade(ctrl),
		Broker:       NewMockCAASBroker(ctrl),
		Logger:       s.logger,
		Clock:        s.clock,
		RegistryFunc: func(i docker.ImageRepoDetails) (registry.Registry, error) { return nil, nil },
	}
	c.Check(cfg.Validate(), jc.ErrorIsNil)
}
