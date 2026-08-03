// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"testing"
	"time"

	"github.com/juju/tc"

	tracingservice "github.com/juju/juju/domain/tracing/service"
)

type runtimeConfigFromWorkloadTracingConfigSuite struct{}

func TestRuntimeConfigFromWorkloadTracingConfigSuite(t *testing.T) {
	tc.Run(t, &runtimeConfigFromWorkloadTracingConfigSuite{})
}

// TestDisabledWhenNoEndpoints asserts that tracing is disabled when neither
// the gRPC nor the HTTP endpoint is configured.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestDisabledWhenNoEndpoints(c *tc.C) {
	cfg := tracingservice.WorkloadTracingConfig{}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.Enabled, tc.IsFalse)
	c.Check(runtimeCfg.HTTPEndpoint, tc.Equals, "")
	c.Check(runtimeCfg.GRPCEndpoint, tc.Equals, "")
}

// TestEnabledWhenGRPCEndpointSet asserts that tracing is enabled when only
// the gRPC endpoint is configured.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestEnabledWhenGRPCEndpointSet(c *tc.C) {
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint: "grpc://otel:4317",
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.Enabled, tc.IsTrue)
	c.Check(runtimeCfg.GRPCEndpoint, tc.Equals, "grpc://otel:4317")
	c.Check(runtimeCfg.HTTPEndpoint, tc.Equals, "")
}

// TestEnabledWhenHTTPEndpointSet asserts that tracing is enabled when only
// the HTTP endpoint is configured.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestEnabledWhenHTTPEndpointSet(c *tc.C) {
	cfg := tracingservice.WorkloadTracingConfig{
		HTTPEndpoint: "http://otel:4318",
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.Enabled, tc.IsTrue)
	c.Check(runtimeCfg.HTTPEndpoint, tc.Equals, "http://otel:4318")
	c.Check(runtimeCfg.GRPCEndpoint, tc.Equals, "")
}

// TestEnabledWhenBothEndpointsSet asserts that tracing is enabled and both
// endpoints are preserved when both are configured.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestEnabledWhenBothEndpointsSet(c *tc.C) {
	cfg := tracingservice.WorkloadTracingConfig{
		HTTPEndpoint:  "http://otel:4318",
		GRPCEndpoint:  "grpc://otel:4317",
		CACertificate: "ca-cert",
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.Enabled, tc.IsTrue)
	c.Check(runtimeCfg.HTTPEndpoint, tc.Equals, "http://otel:4318")
	c.Check(runtimeCfg.GRPCEndpoint, tc.Equals, "grpc://otel:4317")
	c.Check(runtimeCfg.CACertificate, tc.Equals, "ca-cert")
}

// TestDefaultsApplied asserts that the default sample ratio and tail sampling
// threshold are applied when no overrides are provided.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestDefaultsApplied(c *tc.C) {
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint: "grpc://otel:4317",
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.SampleRatio, tc.Equals, defaultOpenTelemetrySampleRatio)
	c.Check(runtimeCfg.TailSamplingThreshold, tc.Equals, defaultOpenTelemetryTailSamplingThreshold)
}

// TestStackTracesEnabled asserts that the stack traces flag is propagated when
// set.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestStackTracesEnabled(c *tc.C) {
	stackTraces := true
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:             "grpc://otel:4317",
		OpenTelemetryStackTraces: &stackTraces,
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.StackTracesEnabled, tc.IsTrue)
}

// TestStackTracesDisabled asserts that the stack traces flag is propagated as
// false when explicitly disabled.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestStackTracesDisabled(c *tc.C) {
	stackTraces := false
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:             "grpc://otel:4317",
		OpenTelemetryStackTraces: &stackTraces,
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.StackTracesEnabled, tc.IsFalse)
}

// TestInsecureSkipVerify asserts that the insecure skip verify flag is
// propagated when set.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestInsecureSkipVerify(c *tc.C) {
	insecure := true
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:       "grpc://otel:4317",
		InsecureSkipVerify: &insecure,
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.InsecureSkipVerify, tc.IsTrue)
}

// TestSampleRatioOverride asserts that a valid sample ratio override is
// applied.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestSampleRatioOverride(c *tc.C) {
	sampleRatio := 0.5
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:             "grpc://otel:4317",
		OpenTelemetrySampleRatio: &sampleRatio,
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.SampleRatio, tc.Equals, 0.5)
}

// TestSampleRatioTooLow asserts that a sample ratio below zero is rejected.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestSampleRatioTooLow(c *tc.C) {
	sampleRatio := -0.1
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:             "grpc://otel:4317",
		OpenTelemetrySampleRatio: &sampleRatio,
	}

	_, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Check(err, tc.ErrorMatches, "open telemetry sample ratio -0.1000 not valid")
}

// TestSampleRatioTooHigh asserts that a sample ratio above one is rejected.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestSampleRatioTooHigh(c *tc.C) {
	sampleRatio := 1.1
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:             "grpc://otel:4317",
		OpenTelemetrySampleRatio: &sampleRatio,
	}

	_, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Check(err, tc.ErrorMatches, "open telemetry sample ratio 1.1000 not valid")
}

// TestTailSamplingThresholdOverride asserts that a valid tail sampling
// threshold override is applied.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestTailSamplingThresholdOverride(c *tc.C) {
	threshold := "5s"
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:                       "grpc://otel:4317",
		OpenTelemetryTailSamplingThreshold: &threshold,
	}

	runtimeCfg, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.TailSamplingThreshold, tc.Equals, 5*time.Second)
}

// TestTailSamplingThresholdInvalid asserts that an invalid tail sampling
// threshold is rejected.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestTailSamplingThresholdInvalid(c *tc.C) {
	threshold := "not-a-duration"
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:                       "grpc://otel:4317",
		OpenTelemetryTailSamplingThreshold: &threshold,
	}

	_, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Check(err, tc.ErrorMatches, `parsing open telemetry tail sampling threshold "not-a-duration".*`)
}

// TestTailSamplingThresholdNegative asserts that a negative tail sampling
// threshold is rejected.
func (s *runtimeConfigFromWorkloadTracingConfigSuite) TestTailSamplingThresholdNegative(c *tc.C) {
	threshold := "-1s"
	cfg := tracingservice.WorkloadTracingConfig{
		GRPCEndpoint:                       "grpc://otel:4317",
		OpenTelemetryTailSamplingThreshold: &threshold,
	}

	_, err := runtimeConfigFromWorkloadTracingConfig(cfg)
	c.Check(err, tc.ErrorMatches, `open telemetry tail sampling threshold "-1s" not valid`)
}
