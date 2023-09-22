// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	jujutesting "github.com/juju/testing"
	"go.opentelemetry.io/otel"
	trace "go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package trace -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package trace -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package trace -destination tracer_mock_test.go github.com/juju/juju/worker/trace TrackedTracer,Client,ClientTracer,ClientTracerProvider
//go:generate go run go.uber.org/mock/mockgen -package trace -destination trace_mock_test.go go.opentelemetry.io/otel/trace Span

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger Logger

	clock                *MockClock
	agent                *MockAgent
	config               *MockConfig
	client               *MockClient
	clientTracer         *MockClientTracer
	clientTracerProvider *MockClientTracerProvider
	span                 *MockSpan
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)

	s.client = NewMockClient(ctrl)
	s.clientTracer = NewMockClientTracer(ctrl)
	s.clientTracerProvider = NewMockClientTracerProvider(ctrl)
	s.span = NewMockSpan(ctrl)

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

func (s *baseSuite) expectClient() {
	s.client.EXPECT().Start(gomock.Any()).AnyTimes()
	s.client.EXPECT().Stop(gomock.Any()).AnyTimes()

	s.clientTracer.EXPECT().Start(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
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
