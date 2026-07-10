// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.AgentConfigChanged = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTracerWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Kind = coretrace.Kind("")
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              s.clock,
		Logger:             s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			return nil, nil
		},
		Kind: coretrace.KindController,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent": s.agent,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	test := func(enabled bool) {
		defer s.setupMocks(c).Finish()

		s.expectCurrentConfig(enabled)
		s.expectOpenTelemetry()

		w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
		c.Assert(err, tc.ErrorIsNil)
		workertest.CleanKill(c, w)
	}

	// Test the noop and real tracer.
	for _, enabled := range []bool{true, false} {
		c.Logf("enabled: %v", enabled)
		test(enabled)
	}
}

func (s *manifoldSuite) TestUnitRuntimeConfigProviderReloadsFromAgentConfigChanged(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	updatedConfig := NewMockConfig(ctrl)
	configChanged := voyeur.NewValue(false)
	useUpdatedConfig := false

	s.agent.EXPECT().CurrentConfig().DoAndReturn(func() coreagent.Config {
		if useUpdatedConfig {
			return updatedConfig
		}
		return s.config
	}).AnyTimes()

	s.expectRuntimeConfig(s.config, RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "http://one:4318/v1/traces",
		InsecureSkipVerify:    true,
		SampleRatio:           0.5,
		TailSamplingThreshold: time.Millisecond,
	})
	s.expectRuntimeConfig(updatedConfig, RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "http://two:4318/v1/traces",
		InsecureSkipVerify:    true,
		StackTracesEnabled:    true,
		SampleRatio:           1,
		TailSamplingThreshold: time.Second,
	})

	provider := unitRuntimeConfigProvider{
		agent:              s.agent,
		agentConfigChanged: configChanged,
	}
	watcher, err := provider.WatchRuntimeConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher)

	initial, err := provider.CurrentRuntimeConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initial, tc.DeepEquals, RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "http://one:4318/v1/traces",
		InsecureSkipVerify:    true,
		SampleRatio:           0.5,
		TailSamplingThreshold: time.Millisecond,
	})

	select {
	case <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for initial config notification")
	}

	useUpdatedConfig = true
	configChanged.Set(true)

	select {
	case <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for config changed notification")
	}

	updated, err := provider.CurrentRuntimeConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updated, tc.DeepEquals, RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "http://two:4318/v1/traces",
		InsecureSkipVerify:    true,
		StackTracesEnabled:    true,
		SampleRatio:           1,
		TailSamplingThreshold: time.Second,
	})
}

func (s *manifoldSuite) expectOpenTelemetry() {
	s.config.EXPECT().Tag().Return(names.NewControllerAgentTag("0"))
	s.config.EXPECT().OpenTelemetryHTTPEndpoint().Return("http://blah:4318").AnyTimes()
	s.config.EXPECT().OpenTelemetryGRPCEndpoint().Return("blah:4317").AnyTimes()
	s.config.EXPECT().OpenTelemetryInsecure().Return(false).AnyTimes()
	s.config.EXPECT().OpenTelemetryStackTraces().Return(true).AnyTimes()
	s.config.EXPECT().OpenTelemetrySampleRatio().Return(0.5).AnyTimes()
	s.config.EXPECT().OpenTelemetryTailSamplingThreshold().Return(time.Second).AnyTimes()
}

func (s *manifoldSuite) expectRuntimeConfig(config *MockConfig, runtimeConfig RuntimeConfig) {
	config.EXPECT().OpenTelemetryEnabled().Return(runtimeConfig.Enabled).AnyTimes()
	config.EXPECT().OpenTelemetryHTTPEndpoint().Return(runtimeConfig.HTTPEndpoint).AnyTimes()
	config.EXPECT().OpenTelemetryGRPCEndpoint().Return(runtimeConfig.GRPCEndpoint).AnyTimes()
	config.EXPECT().OpenTelemetryInsecure().Return(runtimeConfig.InsecureSkipVerify).AnyTimes()
	config.EXPECT().OpenTelemetryStackTraces().Return(runtimeConfig.StackTracesEnabled).AnyTimes()
	config.EXPECT().OpenTelemetrySampleRatio().Return(runtimeConfig.SampleRatio).AnyTimes()
	config.EXPECT().OpenTelemetryTailSamplingThreshold().Return(runtimeConfig.TailSamplingThreshold).AnyTimes()
}
