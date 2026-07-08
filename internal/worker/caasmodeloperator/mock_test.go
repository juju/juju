// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"context"
	"time"

	"github.com/juju/juju/controller"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	coretesting "github.com/juju/juju/internal/testing"
)

type mockConfigProvider struct{}

type mockTracingService struct{}

func (m *mockConfigProvider) CACert() (string, error) {
	return coretesting.CACert, nil
}

func (m *mockConfigProvider) ControllerAgentInfo() (controller.ControllerAgentInfo, error) {
	return controller.ControllerAgentInfo{
		Cert:         "controller-cert",
		PrivateKey:   "controller-key",
		CAPrivateKey: "ca-key",
	}, nil
}

func (m *mockTracingService) GetWorkloadTracingConfig(context.Context) (tracingservice.WorkloadTracingConfig, error) {
	sampleRatio := 0.1000
	tailSamplingThreshold := time.Millisecond.String()
	return tracingservice.WorkloadTracingConfig{
		OpenTelemetrySampleRatio:           &sampleRatio,
		OpenTelemetryTailSamplingThreshold: &tailSamplingThreshold,
	}, nil
}
