// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"time"

	coretesting "github.com/juju/juju/internal/testing"
)

type mockConfigProvider struct{}

func (m *mockConfigProvider) CACert() (string, error) {
	return coretesting.CACert, nil
}

func (m *mockConfigProvider) OpenTelemetryEnabled() bool {
	return false
}

func (m *mockConfigProvider) OpenTelemetryEndpoint() string {
	return ""
}

func (m *mockConfigProvider) OpenTelemetryInsecure() bool {
	return false
}

func (m *mockConfigProvider) OpenTelemetryStackTraces() bool {
	return false
}

func (m *mockConfigProvider) OpenTelemetrySampleRatio() float64 {
	return 0.1000
}

func (m *mockConfigProvider) OpenTelemetryTailSamplingThreshold() time.Duration {
	return time.Millisecond
}
