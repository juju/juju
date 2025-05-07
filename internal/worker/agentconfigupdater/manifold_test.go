// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpubsub "github.com/juju/juju/internal/pubsub"
	"github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/rpc/params"
)

type AgentConfigUpdaterSuite struct {
	testing.BaseSuite
	manifold dependency.Manifold
	hub      *pubsub.StructuredHub
}

var _ = tc.Suite(&AgentConfigUpdaterSuite{})

func (s *AgentConfigUpdaterSuite) SetUpTest(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	s.manifold = agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
		AgentName:      "agent",
		APICallerName:  "api-caller",
		CentralHubName: "central-hub",
		TraceName:      "trace",
		Logger:         logger,
	})
	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Logger: internalpubsub.WrapLogger(logger),
	})
}

func (s *AgentConfigUpdaterSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{
		"agent",
		"api-caller",
		"central-hub",
		"trace",
	})
}

func (s *AgentConfigUpdaterSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestStartAPICallerMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":      &mockAgent{},
		"api-caller": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestNotMachine(c *tc.C) {
	a := &mockAgent{
		conf: mockConfig{tag: names.NewUnitTag("foo/0")},
	}
	getter := dt.StubGetter(map[string]interface{}{
		"agent": a,
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "agent's tag is not a machine or controller agent tag")
}

func (s *AgentConfigUpdaterSuite) TestEntityLookupFailure(c *tc.C) {
	// Set up a fake Agent and APICaller
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Error: &params.Error{Message: "boom"},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	// Call the manifold's start func with a fake resource getter that
	// returns the fake Agent and APICaller
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
		"trace":       coretrace.NoopTracer{},
	})
	w, err := s.manifold.Start(context.Background(), getter)
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "checking controller status: boom")
}

func (s *AgentConfigUpdaterSuite) TestCentralHubMissing(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    1234,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{
						"juju-db-snap-channel":                        controller.DefaultJujuDBSnapChannel,
						"query-tracing-enabled":                       controller.DefaultQueryTracingEnabled,
						"query-tracing-threshold":                     controller.DefaultQueryTracingThreshold,
						controller.OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
						controller.OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
						controller.OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
						controller.OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
						controller.OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
						controller.ObjectStoreType:                    objectstore.FileBackend.String(),
					},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &mockAgent{},
		"api-caller":  apiCaller,
		"central-hub": dependency.ErrMissing,
		"trace":       stubTracerGetter{},
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestCentralHubMissingFirstPass(c *tc.C) {
	agent := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    1234,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       agent,
		"api-caller":  apiCaller,
		"central-hub": dependency.ErrMissing,
		"trace":       stubTracerGetter{},
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *AgentConfigUpdaterSuite) startManifold(c *tc.C, a agent.Agent, mockAPIPort int) (worker.Worker, error) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    mockAPIPort,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{
						"juju-db-snap-channel":                        controller.DefaultJujuDBSnapChannel,
						"query-tracing-enabled":                       controller.DefaultQueryTracingEnabled,
						"query-tracing-threshold":                     controller.DefaultQueryTracingThreshold,
						controller.OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
						controller.OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
						controller.OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
						controller.OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
						controller.OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,
						controller.ObjectStoreType:                    objectstore.FileBackend.String(),
					},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
		"trace":       stubTracerGetter{},
	})
	return s.manifold.Start(context.Background(), getter)
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnviron(c *tc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, tc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, tc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, tc.Equals, "cert")
	c.Assert(a.conf.ssi.PrivateKey, tc.Equals, "key")
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnvironNotOverwriteCert(c *tc.C) {
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
	c.Assert(w, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, tc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, tc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, tc.Equals, existingCert)
	c.Assert(a.conf.ssi.PrivateKey, tc.Equals, existingKey)
}

func (s *AgentConfigUpdaterSuite) TestJobHostUnits(c *tc.C) {
	// State serving info should not be set for JobHostUnits.
	s.checkNotController(c, model.JobHostUnits)
}

func (s *AgentConfigUpdaterSuite) checkNotController(c *tc.C, job model.MachineJob) {
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
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
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.Equals, dependency.ErrUninstall)

	// State serving info shouldn't have been set for this job type.
	c.Assert(a.conf.ssiSet, tc.IsFalse)
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
