// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

type workerConfigSuite struct {
	testing.IsolationSuite

	config workerConfig
}

var _ = gc.Suite(&workerConfigSuite{})

func (s *workerConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = workerConfig{
		statePool:               &state.StatePool{},
		controllerConfigService: &managedServices{},
		userService:             &managedServices{},
		mux:                     &apiserverhttp.Mux{},
		clock:                   clock.WallClock,
		newStateAuthenticatorFn: NewStateAuthenticator,
	}
}

func (s *workerConfigSuite) TestConfigValid(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
}

func (s *workerConfigSuite) TestMissing(c *gc.C) {
	tests := []struct {
		fn       func(workerConfig) workerConfig
		expected string
	}{{
		fn: func(cfg workerConfig) workerConfig {
			cfg.statePool = nil
			return cfg
		},
		expected: "empty statePool",
	}}
	for _, test := range tests {
		cfg := test.fn(s.config)
		err := cfg.Validate()
		c.Assert(err, jc.ErrorIs, errors.NotValid)
	}
}

type workerSuite struct {
	testing.IsolationSuite

	controllerConfigService *MockControllerConfigService
	userService             *MockUserService

	stateAuthFunc func(context.Context, *state.StatePool, ControllerConfigService, UserService, BakeryConfigService, *apiserverhttp.Mux, clock.Clock, <-chan struct{}) (macaroon.LocalMacaroonAuthenticator, error)
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerStarted(c *gc.C) {
	started := make(chan struct{})
	s.stateAuthFunc = func(context.Context, *state.StatePool, ControllerConfigService, UserService, BakeryConfigService, *apiserverhttp.Mux, clock.Clock, <-chan struct{}) (macaroon.LocalMacaroonAuthenticator, error) {
		defer close(started)
		return nil, nil
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerControllerConfigContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, nil)

	started := make(chan struct{})
	s.stateAuthFunc = func(context.Context, *state.StatePool, ControllerConfigService, UserService, BakeryConfigService, *apiserverhttp.Mux, clock.Clock, <-chan struct{}) (macaroon.LocalMacaroonAuthenticator, error) {
		defer close(started)
		return nil, nil
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	config, err := w.(*argsWorker).managedServices.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config, gc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerControllerConfigContextDeadline(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
		return nil, ctx.Err()
	})

	started := make(chan struct{})
	s.stateAuthFunc = func(context.Context, *state.StatePool, ControllerConfigService, UserService, BakeryConfigService, *apiserverhttp.Mux, clock.Clock, <-chan struct{}) (macaroon.LocalMacaroonAuthenticator, error) {
		defer close(started)
		return nil, nil
	}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)

	_, err := w.(*argsWorker).managedServices.ControllerConfig(context.Background())
	c.Assert(err, gc.Equals, context.Canceled)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(context.Background(), s.newWorkerConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) newWorkerConfig(c *gc.C) workerConfig {
	services := &managedServices{
		controllerConfigService: s.controllerConfigService,
		userService:             s.userService,
	}
	return workerConfig{
		statePool:               &state.StatePool{},
		controllerConfigService: services,
		userService:             services,
		mux:                     &apiserverhttp.Mux{},
		clock:                   clock.WallClock,
		newStateAuthenticatorFn: s.stateAuthFunc,
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.userService = NewMockUserService(ctrl)

	return ctrl
}
