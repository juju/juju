// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	jujutesting "github.com/juju/testing"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package tracing -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package tracing -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger Logger

	clock  *MockClock
	agent  *MockAgent
	config *MockConfig
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)

	s.logger = jujujujutesting.CheckLogger{
		Log: c,
	}

	otel.SetLogger(logr.New(&loggoSink{Logger: s.logger}))

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}

func (s *baseSuite) expectCurrentConfig(enabled bool) {
	s.config.EXPECT().OpenTelemetryEnabled().Return(enabled)
	s.agent.EXPECT().CurrentConfig().Return(s.config)
}
