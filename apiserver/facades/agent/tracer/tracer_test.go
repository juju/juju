// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer_test

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/tracer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/rpc/params"
)

var (
	defaultMachineTag = names.NewMachineTag("0")
)

type tracerSuite struct {
	tracerAPI  *tracer.TracerAPI
	authorizer apiservertesting.FakeAuthorizer

	watcherRegistry *facademocks.MockWatcherRegistry
	tracingService  *MockControllerTracingConfigService
}

func TestTracerSuite(t *testing.T) {
	tc.Run(t, &tracerSuite{})
}

func (s *tracerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.tracingService = NewMockControllerTracingConfigService(ctrl)
	return ctrl
}

func (s *tracerSuite) setupAPI(c *tc.C) {
	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: defaultMachineTag,
	}
	s.tracerAPI, err = tracer.NewTracerAPI(s.authorizer, s.watcherRegistry, s.tracingService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *tracerSuite) TestNewTracerAPIRefusesNonAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("some-user"),
	}
	s.tracerAPI, err = tracer.NewTracerAPI(s.authorizer, s.watcherRegistry, s.tracingService)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *tracerSuite) TestNewTracerAPIAcceptsUnitAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("germany/7"),
	}
	s.tracerAPI, err = tracer.NewTracerAPI(s.authorizer, s.watcherRegistry, s.tracingService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *tracerSuite) TestNewTracerAPIAcceptsApplicationAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("germany"),
	}
	s.tracerAPI, err = tracer.NewTracerAPI(s.authorizer, s.watcherRegistry, s.tracingService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *tracerSuite) TestGetControllerTracingConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	args := params.Entity{Tag: "machine-12354"}
	result := s.tracerAPI.GetControllerTracingConfig(c.Context(), args)
	c.Assert(result.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	s.tracingService.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Times(0)
}

func (s *tracerSuite) TestGetControllerTracingConfigForAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	insecureFalse := false
	stackTraces := true
	sampleRatio := 0.5
	tailSamplingThreshold := "1s"
	s.tracingService.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(tracingservice.WorkloadTracingConfig{
		HTTPEndpoint:                       "https://otel.example.com",
		GRPCEndpoint:                       "otel.example.com:4317",
		CACertificate:                      "ca-cert",
		InsecureSkipVerify:                 &insecureFalse,
		OpenTelemetryStackTraces:           &stackTraces,
		OpenTelemetrySampleRatio:           &sampleRatio,
		OpenTelemetryTailSamplingThreshold: &tailSamplingThreshold,
	}, nil)

	args := params.Entity{Tag: defaultMachineTag.String()}
	result := s.tracerAPI.GetControllerTracingConfig(c.Context(), args)
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.HTTPEndpoint, tc.Equals, "https://otel.example.com")
	c.Check(result.GRPCEndpoint, tc.Equals, "otel.example.com:4317")
	c.Assert(result.CACert, tc.NotNil)
	c.Check(*result.CACert, tc.Equals, "ca-cert")
	c.Check(result.InsecureSkipVerify, tc.NotNil)
	c.Check(*result.InsecureSkipVerify, tc.Equals, false)
	c.Check(result.StackTraces, tc.NotNil)
	c.Check(*result.StackTraces, tc.Equals, true)
	c.Check(result.SampleRatio, tc.NotNil)
	c.Check(*result.SampleRatio, tc.Equals, 0.5)
	c.Check(result.TailSamplingThreshold, tc.NotNil)
	c.Check(*result.TailSamplingThreshold, tc.Equals, "1s")
}

func (s *tracerSuite) TestGetControllerTracingConfigEmpty(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.tracingService.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(tracingservice.WorkloadTracingConfig{}, nil)

	args := params.Entity{Tag: defaultMachineTag.String()}
	result := s.tracerAPI.GetControllerTracingConfig(c.Context(), args)
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.HTTPEndpoint, tc.Equals, "")
	c.Check(result.GRPCEndpoint, tc.Equals, "")
	c.Check(result.CACert, tc.IsNil)
	c.Check(result.InsecureSkipVerify, tc.IsNil)
	c.Check(result.StackTraces, tc.IsNil)
	c.Check(result.SampleRatio, tc.IsNil)
	c.Check(result.TailSamplingThreshold, tc.IsNil)
}

func (s *tracerSuite) TestWatchControllerTracingConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	notifyCh := make(chan struct{}, 1)
	notifyCh <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(notifyCh)
	s.tracingService.EXPECT().WatchWorkloadTracingConfig(gomock.Any()).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)

	args := params.Entity{Tag: defaultMachineTag.String()}
	result := s.tracerAPI.WatchControllerTracingConfig(c.Context(), args)
	c.Assert(result.NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Assert(result.Error, tc.IsNil)
}

func (s *tracerSuite) TestWatchControllerTracingConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.tracingService.EXPECT().WatchWorkloadTracingConfig(gomock.Any()).Times(0)

	args := params.Entity{Tag: "machine-12354"}
	result := s.tracerAPI.WatchControllerTracingConfig(c.Context(), args)
	c.Assert(result.NotifyWatcherId, tc.Equals, "")
	c.Assert(result.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

// Ensure the mock satisfies the interface at compile time.
var _ tracer.ControllerTracingConfigService = (*MockControllerTracingConfigService)(nil)
