// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/juju/tc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package trace -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package trace -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package trace -destination tracer_mock_test.go github.com/juju/juju/internal/worker/trace TrackedTracer,Client,ClientTracer,ClientTracerProvider
//go:generate go run go.uber.org/mock/mockgen -typed -package trace -destination trace_mock_test.go go.opentelemetry.io/otel/trace Span

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	clock                *MockClock
	agent                *MockAgent
	config               *MockConfig
	client               *MockClient
	clientTracer         *MockClientTracer
	clientTracerProvider *MockClientTracerProvider
	span                 *MockSpan
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)

	s.client = NewMockClient(ctrl)
	s.clientTracer = NewMockClientTracer(ctrl)
	s.clientTracerProvider = NewMockClientTracerProvider(ctrl)
	s.span = NewMockSpan(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	otel.SetLogger(logr.New(&loggerSink{Logger: s.logger}))

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

func (s *baseSuite) expectClient() {
	s.client.EXPECT().Start(gomock.Any()).AnyTimes()
	s.client.EXPECT().Stop(gomock.Any()).AnyTimes()

	s.clientTracer.EXPECT().Start(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, ClientSpan) {
		return ctx, s.span
	}).AnyTimes()

	s.clientTracerProvider.EXPECT().ForceFlush(gomock.Any()).AnyTimes()
	s.clientTracerProvider.EXPECT().Shutdown(gomock.Any()).AnyTimes()

	// SetAttributes is used to annotate all spans.
	s.span.EXPECT().SetAttributes(gomock.Any()).AnyTimes()

	// SpanContext is used to get the id for all span calls.
	s.span.EXPECT().SpanContext().Return(trace.SpanContext{}).AnyTimes()

	// End and IsRecording is used at the end of all span calls.
	s.span.EXPECT().End(gomock.Any()).AnyTimes()
	s.span.EXPECT().IsRecording().Return(true).AnyTimes()
}
