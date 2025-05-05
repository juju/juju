// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/rpc/params"
)

type AgentConfigUpdaterSuite struct {
	testing.BaseSuite

	manifold dependency.Manifold

	controllerDomainServices *MockControllerDomainServices
	controllerNodeService    *MockControllerNodeService
	controllerConfigService  *MockControllerConfigService
}

var _ = gc.Suite(&AgentConfigUpdaterSuite{})

func (s *AgentConfigUpdaterSuite) TestInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupManifold(c)

	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"agent",
		"api-caller",
		"domain-services",
		"trace",
	})
}

func (s *AgentConfigUpdaterSuite) TestStartAgentMissing(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupManifold(c)

	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestStartAPICallerMissing(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupManifold(c)

	getter := dt.StubGetter(map[string]interface{}{
		"agent":           &mockAgent{},
		"domain-services": s.controllerDomainServices,
		"api-caller":      dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestNotMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupManifold(c)

	a := &mockAgent{
		conf: mockConfig{tag: names.NewUnitTag("foo/0")},
	}
	getter := dt.StubGetter(map[string]interface{}{
		"agent": a,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "agent's tag is not a machine or controller agent tag")
}

func (s *AgentConfigUpdaterSuite) TestIsControllerFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupManifold(c)

	// Set up a fake Agent and APICaller
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			return nil
		},
	)

	s.controllerNodeService.EXPECT().IsControllerNode(gomock.Any(), "99").Return(false, errors.New("boom"))

	// Call the manifold's start func with a fake resource getter that
	// returns the fake Agent and APICaller
	getter := dt.StubGetter(map[string]interface{}{
		"agent":           a,
		"api-caller":      apiCaller,
		"domain-services": s.controllerDomainServices,
		"trace":           coretrace.NoopTracer{},
	})
	w, err := s.manifold.Start(context.Background(), getter)
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "checking is controller: boom")
}

func (s *AgentConfigUpdaterSuite) startManifold(c *gc.C, a agent.Agent, mockAPIPort int) (worker.Worker, error) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, gc.Equals, "Agent")
			switch request {
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    mockAPIPort,
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	getter := dt.StubGetter(map[string]interface{}{
		"agent":           a,
		"api-caller":      apiCaller,
		"domain-services": s.controllerDomainServices,
		"trace":           stubTracerGetter{},
	})
	return s.manifold.Start(context.Background(), getter)
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnviron(c *gc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, jc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, gc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, gc.Equals, "cert")
	c.Assert(a.conf.ssi.PrivateKey, gc.Equals, "key")
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnvironNotOverwriteCert(c *gc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	existingCert := "some cert set by certupdater"
	existingKey := "some key set by certupdater"
	a.conf.SetStateServingInfo(controller.StateServingInfo{
		Cert:       existingCert,
		PrivateKey: existingKey,
	})

	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, jc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, gc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, gc.Equals, existingCert)
	c.Assert(a.conf.ssi.PrivateKey, gc.Equals, existingKey)
}

func (s *AgentConfigUpdaterSuite) TestJobHostUnits(c *gc.C) {
	// State serving info should not be set for JobHostUnits.
	s.checkNotController(c, model.JobHostUnits)
}

func (s *AgentConfigUpdaterSuite) checkNotController(c *gc.C, job model.MachineJob) {
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, gc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, gc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{job},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	w, err := s.manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"agent":      a,
		"api-caller": apiCaller,
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)

	// State serving info shouldn't have been set for this job type.
	c.Assert(a.conf.ssiSet, jc.IsFalse)
}

func (s *AgentConfigUpdaterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.controllerDomainServices = NewMockControllerDomainServices(ctrl)

	return ctrl
}

func (s *AgentConfigUpdaterSuite) setupManifold(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	s.manifold = agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		DomainServicesName: "domain-services",
		TraceName:          "trace",
		Logger:             logger,
		GetControllerDomainServicesFn: func(dependency.Getter, string) (agentconfigupdater.ControllerDomainServices, error) {
			return controllerDomainServices{
				ControllerConfigService: s.controllerConfigService,
				ControllerNodeService:   s.controllerNodeService,
			}, nil
		},
	})
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag    names.Tag
	ssiSet bool
	ssi    controller.StateServingInfo

	snapChannel    string
	snapChannelSet bool

	queryTracingEnabled    bool
	queryTracingEnabledSet bool

	queryTracingThreshold    time.Duration
	queryTracingThresholdSet bool

	openTelemetryEnabled    bool
	openTelemetryEnabledSet bool

	openTelemetryEndpoint    string
	openTelemetryEndpointSet bool

	openTelemetryInsecure    bool
	openTelemetryInsecureSet bool

	openTelemetryStackTraces    bool
	openTelemetryStackTracesSet bool

	openTelemetrySampleRatio    float64
	openTelemetrySampleRatioSet bool

	openTelemetryTailSamplingThreshold    time.Duration
	openTelemetryTailSamplingThresholdSet bool

	objectStoreType    objectstore.BackendType
	objectStoreTypeSet bool
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Model() names.ModelTag {
	return testing.ModelTag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) JujuDBSnapChannel() string {
	if mc.snapChannel == "" {
		return controller.DefaultJujuDBSnapChannel
	}
	return mc.snapChannel
}

func (mc *mockConfig) SetJujuDBSnapChannel(snapChannel string) {
	mc.snapChannel = snapChannel
	mc.snapChannelSet = true
}

func (mc *mockConfig) QueryTracingEnabled() bool {
	return mc.queryTracingEnabled
}

func (mc *mockConfig) SetQueryTracingEnabled(enabled bool) {
	mc.queryTracingEnabled = enabled
	mc.queryTracingEnabledSet = true
}

func (mc *mockConfig) QueryTracingThreshold() time.Duration {
	if mc.queryTracingThreshold == 0 {
		return controller.DefaultQueryTracingThreshold
	}
	return mc.queryTracingThreshold
}

func (mc *mockConfig) SetQueryTracingThreshold(threshold time.Duration) {
	mc.queryTracingThreshold = threshold
	mc.queryTracingThresholdSet = true
}

func (mc *mockConfig) OpenTelemetryEnabled() bool {
	return mc.openTelemetryEnabled
}

func (mc *mockConfig) SetOpenTelemetryEnabled(enabled bool) {
	mc.openTelemetryEnabled = enabled
	mc.openTelemetryEnabledSet = true
}

func (mc *mockConfig) OpenTelemetryEndpoint() string {
	return mc.openTelemetryEndpoint
}

func (mc *mockConfig) SetOpenTelemetryEndpoint(endpoint string) {
	mc.openTelemetryEndpoint = endpoint
	mc.openTelemetryEndpointSet = true
}

func (mc *mockConfig) OpenTelemetryInsecure() bool {
	return mc.openTelemetryInsecure
}

func (mc *mockConfig) SetOpenTelemetryInsecure(enabled bool) {
	mc.openTelemetryInsecure = enabled
	mc.openTelemetryInsecureSet = true
}

func (mc *mockConfig) OpenTelemetryStackTraces() bool {
	return mc.openTelemetryStackTraces
}

func (mc *mockConfig) SetOpenTelemetryStackTraces(enabled bool) {
	mc.openTelemetryStackTraces = enabled
	mc.openTelemetryStackTracesSet = true
}

func (mc *mockConfig) OpenTelemetrySampleRatio() float64 {
	if mc.openTelemetrySampleRatio == 0 {
		return controller.DefaultOpenTelemetrySampleRatio
	}
	return mc.openTelemetrySampleRatio
}

func (mc *mockConfig) SetOpenTelemetrySampleRatio(ratio float64) {
	mc.openTelemetrySampleRatio = ratio
	mc.openTelemetrySampleRatioSet = true
}

func (mc *mockConfig) OpenTelemetryTailSamplingThreshold() time.Duration {
	if mc.openTelemetryTailSamplingThreshold == 0 {
		return controller.DefaultOpenTelemetryTailSamplingThreshold
	}
	return mc.openTelemetryTailSamplingThreshold
}

func (mc *mockConfig) SetOpenTelemetryTailSamplingThreshold(dur time.Duration) {
	mc.openTelemetryTailSamplingThreshold = dur
	mc.openTelemetryTailSamplingThresholdSet = true
}

func (mc *mockConfig) ObjectStoreType() objectstore.BackendType {
	if mc.objectStoreType == "" {
		return objectstore.FileBackend
	}
	return mc.objectStoreType
}

func (mc *mockConfig) SetObjectStoreType(value objectstore.BackendType) {
	mc.objectStoreType = value
	mc.objectStoreTypeSet = true
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}

type stubTracerGetter struct {
	trace.TracerGetter
}

func (s stubTracerGetter) GetTracer(context.Context, coretrace.TracerNamespace) (coretrace.Tracer, error) {
	return coretrace.NoopTracer{}, nil
}

type controllerDomainServices struct {
	agentconfigupdater.ControllerConfigService
	agentconfigupdater.ControllerNodeService
}

// ControllerConfigService is an interface that defines the methods that are
// required to get the controller configuration.
func (s controllerDomainServices) ControllerConfig() agentconfigupdater.ControllerConfigService {
	return s.ControllerConfigService
}

// ControllerNodeService is an interface that defines the methods that are
// required to check if a machine or container agent is a controller node.
func (s controllerDomainServices) ControllerNode() agentconfigupdater.ControllerNodeService {
	return s.ControllerNodeService
}
