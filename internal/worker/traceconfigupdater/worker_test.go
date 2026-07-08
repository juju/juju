// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceconfigupdater

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/tracer"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

var defaultTracingConfig = tracer.ControllerTracingConfig{
	HTTPEndpoint: "https://otel.example.com",
	GRPCEndpoint: "otel.example.com:4317",
	CACert:       "ca-cert",
}

type workerSuite struct {
	testhelpers.IsolationSuite

	agent         *MockAgent
	agentConfig   *MockConfig
	api           *MockTracingAPI
	notifyWatcher *MockNotifyWatcher

	tag           names.Tag
	realConfig    agent.ConfigSetterWriter
	configChanged *voyeur.Value
	workerConfig  WorkerConfig
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.tag = names.NewMachineTag("0")
	s.realConfig = newAgentConfig(c)
	s.configChanged = voyeur.NewValue(true)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.api = NewMockTracingAPI(ctrl)
	s.notifyWatcher = NewMockNotifyWatcher(ctrl)

	s.agentConfig.EXPECT().Tag().Return(s.tag).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()

	s.workerConfig = WorkerConfig{
		Agent:              s.agent,
		API:                s.api,
		AgentConfigChanged: s.configChanged,
		Logger:             loggertesting.WrapCheckLog(c),
	}

	c.Cleanup(func() {
		s.agent = nil
		s.agentConfig = nil
		s.api = nil
		s.notifyWatcher = nil
	})

	return ctrl
}

// expectCurrentConfigReads sets up the mock expectations for the read side of
// the current agent config. These are consulted in update() to decide whether
// the config needs to be written.
func (s *workerSuite) expectCurrentConfigReads() {
	s.agentConfig.EXPECT().OpenTelemetryEnabled().Return(false).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryHTTPEndpoint().Return("").AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryGRPCEndpoint().Return("").AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryInsecure().Return(false).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryStackTraces().Return(false).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetrySampleRatio().Return(0.0).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryTailSamplingThreshold().Return(time.Duration(0)).AnyTimes()
}

// expectCurrentConfigReadsMatching sets up reads that match the supplied
// desired config, so that update() short-circuits without writing.
func (s *workerSuite) expectCurrentConfigReadsMatching(r resolvedTracingConfig) {
	s.agentConfig.EXPECT().OpenTelemetryEnabled().Return(r.enabled).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryHTTPEndpoint().Return(r.httpEndpoint).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryGRPCEndpoint().Return(r.grpcEndpoint).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryInsecure().Return(r.insecure).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryStackTraces().Return(r.stackTraces).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetrySampleRatio().Return(r.sampleRatio).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryTailSamplingThreshold().Return(r.tailSamplingThreshold).AnyTimes()
}

// expectChangeConfig sets up ChangeConfig to invoke the mutator with a real
// agent.ConfigSetterWriter so that the setters are exercised and the written
// values can be verified afterwards.
func (s *workerSuite) expectChangeConfig() {
	s.agent.EXPECT().ChangeConfig(gomock.Any()).
		DoAndReturn(func(mutator agent.ConfigMutator) error {
			return mutator(s.realConfig)
		})
}

// expectGetTracingConfig sets up the GetControllerTracingConfig expectation.
func (s *workerSuite) expectGetTracingConfig(config tracer.ControllerTracingConfig, err error) {
	s.api.EXPECT().GetControllerTracingConfig(gomock.Any(), gomock.Any()).
		Return(config, err)
}

// expectWatch sets up the WatchControllerTracingConfig expectation.
func (s *workerSuite) expectWatch() {
	s.api.EXPECT().WatchControllerTracingConfig(gomock.Any(), gomock.Any()).
		Return(s.notifyWatcher, nil)
}

// --- WorkerConfig.Validate tests ---

func (s *workerSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Check(WorkerConfig{}.Validate(), tc.ErrorMatches, "missing agent not valid")

	config := s.workerConfig
	config.API = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing api not valid")

	config = s.workerConfig
	config.AgentConfigChanged = nil
	c.Check(config.Validate(), tc.ErrorMatches, "nil AgentConfigChanged not valid")

	config = s.workerConfig
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing logger not valid")

	c.Check(s.workerConfig.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) TestSetUpPersistsInitialConfigAndStartsWatcher(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetTracingConfig(defaultTracingConfig, nil)
	s.expectCurrentConfigReads()
	s.expectChangeConfig()
	s.expectWatch()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	w, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(w, tc.Equals, s.notifyWatcher)
	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsTrue)
	c.Check(s.realConfig.OpenTelemetryHTTPEndpoint(), tc.Equals, "https://otel.example.com")
	c.Check(s.realConfig.OpenTelemetryGRPCEndpoint(), tc.Equals, "otel.example.com:4317")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestSetUpDoesNotWriteUnchangedConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetTracingConfig(defaultTracingConfig, nil)
	s.expectCurrentConfigReadsMatching(resolveTracingConfig(defaultTracingConfig))
	s.expectWatch()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	_, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsFalse)
	c.Check(s.realConfig.OpenTelemetryHTTPEndpoint(), tc.Equals, "")
	assertNoConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsChangedConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetTracingConfig(defaultTracingConfig, nil)
	s.expectCurrentConfigReads()
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsTrue)
	c.Check(s.realConfig.OpenTelemetryHTTPEndpoint(), tc.Equals, "https://otel.example.com")
	c.Check(s.realConfig.OpenTelemetryGRPCEndpoint(), tc.Equals, "otel.example.com:4317")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsEmptyConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Simulate the agent already having a config, and the controller
	// returning empty (tracing disabled).
	s.realConfig.SetOpenTelemetryEnabled(true)
	s.realConfig.SetOpenTelemetryHTTPEndpoint("https://old-otel.example.com")
	s.realConfig.SetOpenTelemetryGRPCEndpoint("old-otel.example.com:4317")

	s.expectGetTracingConfig(tracer.ControllerTracingConfig{}, nil)
	s.agentConfig.EXPECT().OpenTelemetryEnabled().Return(true).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryHTTPEndpoint().Return("https://old-otel.example.com").AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryGRPCEndpoint().Return("old-otel.example.com:4317").AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryInsecure().Return(false).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryStackTraces().Return(false).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetrySampleRatio().Return(0.0).AnyTimes()
	s.agentConfig.EXPECT().OpenTelemetryTailSamplingThreshold().Return(time.Duration(0)).AnyTimes()
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsFalse)
	c.Check(s.realConfig.OpenTelemetryHTTPEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.OpenTelemetryGRPCEndpoint(), tc.Equals, "")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandleResolvesNilPointerDefaults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	insecure := true
	stackTraces := true
	sampleRatio := 0.25
	tailSamplingThreshold := "5s"
	cfg := tracer.ControllerTracingConfig{
		HTTPEndpoint:          "https://otel.example.com",
		InsecureSkipVerify:    &insecure,
		StackTraces:           &stackTraces,
		SampleRatio:           &sampleRatio,
		TailSamplingThreshold: &tailSamplingThreshold,
	}
	s.expectGetTracingConfig(cfg, nil)
	s.expectCurrentConfigReads()
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsTrue)
	c.Check(s.realConfig.OpenTelemetryInsecure(), tc.IsTrue)
	c.Check(s.realConfig.OpenTelemetryStackTraces(), tc.IsTrue)
	c.Check(s.realConfig.OpenTelemetrySampleRatio(), tc.Equals, 0.25)
	c.Check(s.realConfig.OpenTelemetryTailSamplingThreshold(), tc.Equals, 5*time.Second)
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandleGetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetTracingConfig(tracer.ControllerTracingConfig{}, errors.New("boom"))
	s.expectCurrentConfigReads()

	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "getting controller tracing config: boom")
	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsFalse)
}

func (s *workerSuite) TestHandleChangeConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetTracingConfig(defaultTracingConfig, nil)
	s.expectCurrentConfigReads()
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(errors.New("boom"))

	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "updating agent tracing config: boom")
	c.Check(s.realConfig.OpenTelemetryEnabled(), tc.IsFalse)
}

func (s *workerSuite) newUpdater(c *tc.C) *traceConfigUpdater {
	c.Assert(s.workerConfig.Validate(), tc.ErrorIsNil)
	return &traceConfigUpdater{
		config: s.workerConfig,
		tag:    s.tag,
	}
}

func newAgentConfig(c *tc.C) agent.ConfigSetterWriter {
	cfg, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: c.MkDir(),
			LogDir:  c.MkDir(),
		},
		Jobs:              []model.MachineJob{model.JobHostUnits},
		UpgradedToVersion: semversion.MustParse("4.0.0"),
		Tag:               names.NewMachineTag("0"),
		Password:          "password",
		Nonce:             "nonce",
		Controller:        coretesting.ControllerTag,
		Model:             coretesting.ModelTag,
		APIAddresses:      []string{"127.0.0.1:17070"},
		CACert:            "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func watchConfigChanged(value *voyeur.Value) <-chan bool {
	watcher := value.Watch()
	watcher.Next()
	ch := make(chan bool, 1)
	go func() {
		defer watcher.Close()
		ch <- watcher.Next()
	}()
	return ch
}

func assertConfigChanged(c *tc.C, ch <-chan bool) {
	select {
	case changed := <-ch:
		c.Check(changed, tc.IsTrue)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for config changed signal")
	}
}

func assertNoConfigChanged(c *tc.C, ch <-chan bool) {
	select {
	case <-ch:
		c.Fatalf("unexpected config changed signal")
	case <-time.After(testhelpers.ShortWait):
	}
}
