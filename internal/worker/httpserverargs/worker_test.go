// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type workerConfigSuite struct {
	testhelpers.IsolationSuite

	config workerConfig
}

func TestWorkerConfigSuite(t *testing.T) {
	tc.Run(t, &workerConfigSuite{})
}

func (s *workerConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = workerConfig{
		statePool:               &state.StatePool{},
		controllerConfigService: &managedServices{},
		accessService:           &managedServices{},
		macaroonService:         &managedServices{},
		domainServicesGetter:    &managedServices{},
		mux:                     &apiserverhttp.Mux{},
		clock:                   clock.WallClock,
		newStateAuthenticatorFn: NewStateAuthenticator,
	}
}

func (s *workerConfigSuite) TestConfigValid(c *tc.C) {
	c.Assert(s.config.Validate(), tc.ErrorIsNil)
}

func (s *workerConfigSuite) TestMissing(c *tc.C) {
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
		c.Assert(err, tc.ErrorIs, errors.NotValid)
	}
}

type workerSuite struct {
	statetesting.StateSuite

	domainServicesGetter    *MockDomainServicesGetter
	controllerConfigService *MockControllerConfigService
	accessService           *MockAccessService

	stateAuthFunc NewStateAuthenticatorFunc
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func startedAuthFunc(started chan struct{}) NewStateAuthenticatorFunc {
	return func(
		ctx context.Context,
		statePool *state.StatePool,
		controllerModelUUID model.UUID,
		controllerConfigService ControllerConfigService,
		agentPasswordServiceGetter AgentPasswordServiceGetter,
		accessService AccessService,
		machineServiceGetter MachineServiceGetter,
		macaroonService MacaroonService,
		mux *apiserverhttp.Mux,
		clock clock.Clock,
	) (macaroon.LocalMacaroonAuthenticator, error) {
		defer close(started)
		return nil, nil
	}
}

func (s *workerSuite) TestWorkerStarted(c *tc.C) {
	started := make(chan struct{})
	s.stateAuthFunc = startedAuthFunc(started)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerControllerConfigContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, nil)

	started := make(chan struct{})
	s.stateAuthFunc = startedAuthFunc(started)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	config, err := w.(*argsWorker).managedServices.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerControllerConfigContextDeadline(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (controller.Config, error) {
		return nil, ctx.Err()
	})

	started := make(chan struct{})
	s.stateAuthFunc = startedAuthFunc(started)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)

	_, err := w.(*argsWorker).managedServices.ControllerConfig(c.Context())
	c.Assert(err, tc.Equals, context.Canceled)
}

func (s *workerSuite) TestWorkerServicesForModelContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	type svc struct {
		services.DomainServices
	}

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), gomock.Any()).Return(svc{}, nil)

	started := make(chan struct{})
	s.stateAuthFunc = startedAuthFunc(started)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	config, err := w.(*argsWorker).managedServices.ServicesForModel(c.Context(), "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerServicesForModelContextDeadline(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, u model.UUID) (services.DomainServices, error) {
		return nil, ctx.Err()
	})

	started := make(chan struct{})
	s.stateAuthFunc = startedAuthFunc(started)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)

	_, err := w.(*argsWorker).managedServices.ServicesForModel(c.Context(), "")
	c.Assert(err, tc.Equals, context.Canceled)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := newWorker(s.newWorkerConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) newWorkerConfig(c *tc.C) workerConfig {
	services := &managedServices{
		domainServicesGetter:    s.domainServicesGetter,
		controllerConfigService: s.controllerConfigService,
		accessService:           s.accessService,
	}
	return workerConfig{
		statePool:               s.StatePool,
		domainServicesGetter:    services,
		controllerConfigService: services,
		accessService:           services,
		mux:                     &apiserverhttp.Mux{},
		clock:                   clock.WallClock,
		newStateAuthenticatorFn: s.stateAuthFunc,
	}
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.accessService = NewMockAccessService(ctrl)

	return ctrl
}
