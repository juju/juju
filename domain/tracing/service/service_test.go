// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	st *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetCharmTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := CharmTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{
		httpEndpointKey:  "http://localhost:4318",
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigNoFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{}, []string{
		httpEndpointKey,
		grpcEndpointKey,
		caCertificateKey,
	}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), CharmTracingConfig{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := CharmTracingConfig{
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{
		httpEndpointKey,
	}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf("boom"),
	)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), CharmTracingConfig{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetCharmTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(map[string]string{
		httpEndpointKey:  "http://localhost:4318",
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, nil)

	config, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, CharmTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	})
}

func (s *serviceSuite) TestGetCharmTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(map[string]string{
		grpcEndpointKey: "localhost:4317",
	}, nil)

	config, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, CharmTracingConfig{
		HTTPEndpoint:  "",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "",
	})
}

func (s *serviceSuite) TestGetCharmTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetWorkloadTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetryStackTraces := new(bool)
	insecureSkipVerify := new(bool)
	*insecureSkipVerify = true
	openTelemetrySampleRatio := new(float64)
	*openTelemetrySampleRatio = 0.42
	openTelemetryTailSamplingThreshold := new(string)
	*openTelemetryTailSamplingThreshold = "250ms"

	config := WorkloadTracingConfig{
		HTTPEndpoint:                       "http://localhost:4318",
		GRPCEndpoint:                       "localhost:4317",
		CACertificate:                      "cert-data",
		InsecureSkipVerify:                 insecureSkipVerify,
		OpenTelemetryStackTraces:           openTelemetryStackTraces,
		OpenTelemetrySampleRatio:           openTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
	}

	s.st.EXPECT().SetWorkloadTracingConfig(gomock.Any(), map[string]string{
		httpEndpointKey:                       "http://localhost:4318",
		grpcEndpointKey:                       "localhost:4317",
		caCertificateKey:                      "cert-data",
		insecureSkipVerifyKey:                 "true",
		openTelemetryStackTracesKey:           "false",
		openTelemetrySampleRatioKey:           "0.42",
		openTelemetryTailSamplingThresholdKey: "250ms",
	}, []string{}).Return(nil)

	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigNoFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetWorkloadTracingConfig(gomock.Any(), map[string]string{}, []string{
		httpEndpointKey,
		grpcEndpointKey,
		caCertificateKey,
		insecureSkipVerifyKey,
		openTelemetryStackTracesKey,
		openTelemetrySampleRatioKey,
		openTelemetryTailSamplingThresholdKey,
	}).Return(nil)

	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), WorkloadTracingConfig{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetryStackTraces := new(bool)
	*openTelemetryStackTraces = true
	insecureSkipVerify := new(bool)

	config := WorkloadTracingConfig{
		GRPCEndpoint:             "localhost:4317",
		CACertificate:            "cert-data",
		InsecureSkipVerify:       insecureSkipVerify,
		OpenTelemetryStackTraces: openTelemetryStackTraces,
	}

	s.st.EXPECT().SetWorkloadTracingConfig(gomock.Any(), map[string]string{
		grpcEndpointKey:             "localhost:4317",
		caCertificateKey:            "cert-data",
		insecureSkipVerifyKey:       "false",
		openTelemetryStackTracesKey: "true",
	}, []string{
		httpEndpointKey,
		openTelemetrySampleRatioKey,
		openTelemetryTailSamplingThresholdKey,
	}).Return(nil)

	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigInvalidSampleRatio(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetrySampleRatio := 1.42
	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), WorkloadTracingConfig{
		OpenTelemetrySampleRatio: &openTelemetrySampleRatio,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigInvalidTailSamplingThreshold(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetryTailSamplingThreshold := "not-a-duration"
	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), WorkloadTracingConfig{
		OpenTelemetryTailSamplingThreshold: &openTelemetryTailSamplingThreshold,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigNegativeTailSamplingThreshold(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetryTailSamplingThreshold := "-1s"
	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), WorkloadTracingConfig{
		OpenTelemetryTailSamplingThreshold: &openTelemetryTailSamplingThreshold,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetWorkloadTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetWorkloadTracingConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf("boom"),
	)

	err := NewService(s.st).SetWorkloadTracingConfig(c.Context(), WorkloadTracingConfig{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetWorkloadTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(map[string]string{
		httpEndpointKey:                       "http://localhost:4318",
		grpcEndpointKey:                       "localhost:4317",
		caCertificateKey:                      "cert-data",
		insecureSkipVerifyKey:                 "true",
		openTelemetryStackTracesKey:           "true",
		openTelemetrySampleRatioKey:           "0.5",
		openTelemetryTailSamplingThresholdKey: "123ms",
	}, nil)

	insecureSkipVerify := new(bool)
	*insecureSkipVerify = true
	openTelemetryStackTraces := new(bool)
	*openTelemetryStackTraces = true
	openTelemetrySampleRatio := new(float64)
	*openTelemetrySampleRatio = 0.5
	openTelemetryTailSamplingThreshold := new(string)
	*openTelemetryTailSamplingThreshold = "123ms"

	config, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, WorkloadTracingConfig{
		HTTPEndpoint:                       "http://localhost:4318",
		GRPCEndpoint:                       "localhost:4317",
		CACertificate:                      "cert-data",
		InsecureSkipVerify:                 insecureSkipVerify,
		OpenTelemetryStackTraces:           openTelemetryStackTraces,
		OpenTelemetrySampleRatio:           openTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
	})
}

func (s *serviceSuite) TestGetWorkloadTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(map[string]string{
		grpcEndpointKey: "localhost:4317",
	}, nil)

	config, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, WorkloadTracingConfig{
		HTTPEndpoint:  "",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "",
	})
}

func (s *serviceSuite) TestGetWorkloadTracingConfigInvalidStackTraces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(map[string]string{
		openTelemetryStackTracesKey: "not-a-bool",
	}, nil)

	_, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "parsing .*open-telemetry-stack-traces.*")
}

func (s *serviceSuite) TestGetWorkloadTracingConfigInvalidInsecureSkipVerify(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(map[string]string{
		insecureSkipVerifyKey: "not-a-bool",
	}, nil)

	_, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "parsing .*insecure-skip-verify.*")
}

func (s *serviceSuite) TestGetWorkloadTracingConfigInvalidSampleRatio(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(map[string]string{
		openTelemetrySampleRatioKey: "not-a-float",
	}, nil)

	_, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "parsing .*open-telemetry-sample-ratio.*")
}

func (s *serviceSuite) TestGetWorkloadTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetWorkloadTracingConfig(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := NewService(s.st).GetWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	c.Cleanup(func() { s.st = nil })

	return ctrl
}
