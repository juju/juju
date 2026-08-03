// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	time "time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.opentelemetry.io/otel/trace"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	internaltesting "github.com/juju/juju/internal/testing"
)

var testCACert = internaltesting.CACert

type tracerSuite struct {
	baseSuite
}

func TestTracerSuite(t *testing.T) {
	tc.Run(t, &tracerSuite{})
}

var _ coretrace.Tracer = (*tracer)(nil)

func (s *tracerSuite) TestNewClientRejectsInvalidCACertificate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	_, _, _, err := NewClient(
		c.Context(),
		ns,
		"", "localhost:4317",
		"not a pem certificate",
		false,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorMatches, "failed to append trace CA cert to pool")
}

func (s *tracerSuite) TestNewClientRejectsInvalidCACertificateHTTPEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	_, _, _, err := NewClient(
		c.Context(),
		ns,
		"http://otel.example.com:4318/v1/traces", "",
		"not a pem certificate",
		false,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorMatches, "failed to append trace CA cert to pool")
}

func (s *tracerSuite) TestIsHTTPEndpoint(c *tc.C) {
	tests := []struct {
		endpoint string
		expected bool
	}{
		{"http://localhost:4318/v1/traces", true},
		{"https://otel.example.com:4318/v1/traces", true},
		{"http://otel.example.com:4318", true},
		{"localhost:4317", false},
		{"otel.example.com:4317", false},
		{"grpc://otel.example.com:4317", false},
		{"", false},
	}
	for _, test := range tests {
		c.Check(isHTTPEndpoint(test.endpoint), tc.Equals, test.expected, tc.Commentf("endpoint %q", test.endpoint))
	}
}

func (s *tracerSuite) TestNewClientWithGRPCEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"", "localhost:4317",
		"",
		true,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientWithHTTPEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"http://otel.example.com:4318/v1/traces", "",
		"",
		true,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientWithHTTPSEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"https://otel.example.com:4318/v1/traces", "",
		"",
		true,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientWithHTTPSEndpointAndCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"https://otel.example.com:4318/v1/traces", "",
		testCACert,
		false,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientWithGRPCEndpointAndCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"", "localhost:4317",
		testCACert,
		false,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientPrefersGRPCOverHTTP(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	// When both endpoints are provided, the gRPC endpoint should be
	// used (preferred over HTTP). We can't easily assert the exporter
	// type, but we verify the client is created without error.
	client, _, _, err := NewClient(
		c.Context(),
		ns,
		"http://otel.example.com:4318", "localhost:4317",
		"",
		true,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *tracerSuite) TestNewClientNoEndpointReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	_, _, _, err := NewClient(
		c.Context(),
		ns,
		"", "",
		"",
		true,
		0.42,
		time.Second,
		s.logger,
	)
	c.Assert(err, tc.ErrorMatches, "no valid endpoint provided .*")
}

func (s *tracerSuite) TestTracer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(c.Context(), "foo")
	c.Check(ctx, tc.NotNil)
	c.Check(span, tc.NotNil)

	defer span.End()
}

func (s *tracerSuite) TestTracerStartContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	select {
	case <-ctx.Done():
		c.Fatalf("context should not be done")
	default:
	}
}

func (s *tracerSuite) TestTracerStartAfterKilledReturnsNoop(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	workertest.DirtyKill(c, tracer)

	parentCtx := c.Context()
	ctx, span := tracer.Start(parentCtx, "foo")
	c.Check(ctx, tc.Equals, parentCtx)
	c.Check(ctx.Err(), tc.ErrorIsNil)

	_, ok := span.(coretrace.NoopSpan)
	c.Check(ok, tc.IsTrue)
}

func (s *tracerSuite) TestTracerStartContextNotCanceledWhenTracerDies(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)

	ctx, span := tracer.Start(c.Context(), "foo")
	workertest.DirtyKill(c, tracer)
	defer span.End()

	select {
	case <-ctx.Done():
		c.Fatalf("context should not be done")
	default:
	}
}

func (s *tracerSuite) TestTracerStartContextCarriesCoreSpan(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	_, ok := coretrace.SpanFromContext(ctx).(*limitedSpan)
	c.Check(ok, tc.IsTrue)
}

func (s *tracerSuite) TestTracerStartReturnsBuiltRequestContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx := coretrace.WithTraceScope(c.Context(), "80f198ee56343ba864fe8b2a57d3eff7", "ff00000000000000", 1)
	ctx, span := tracer.Start(ctx, "foo")
	defer span.End()

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Check(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)

	traceID, ok = coretrace.TraceIDFromContext(ctx)
	c.Check(ok, tc.IsTrue)
	c.Check(traceID, tc.Equals, "80f198ee56343ba864fe8b2a57d3eff7")
}

func (s *tracerSuite) TestContextWithSpanCarriesTraceValues(c *tc.C) {
	traceID, err := trace.TraceIDFromHex("80f198ee56343ba864fe8b2a57d3eff7")
	c.Assert(err, tc.ErrorIsNil)
	spanID, err := trace.SpanIDFromHex("ff00000000000000")
	c.Assert(err, tc.ErrorIsNil)

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
	otelCtx := trace.ContextWithSpanContext(c.Context(), spanContext)
	otelSpan := trace.SpanFromContext(otelCtx)
	coreSpan := coretrace.NoopSpan{}

	ctx := contextWithSpan(c.Context(), otelSpan, coreSpan)
	c.Check(ctx.Err(), tc.ErrorIsNil)
	c.Check(coretrace.SpanFromContext(ctx), tc.Equals, coreSpan)

	gotTraceID, ok := coretrace.TraceIDFromContext(ctx)
	c.Check(ok, tc.IsTrue)
	c.Check(gotTraceID, tc.Equals, traceID.String())
	c.Check(trace.SpanFromContext(ctx).SpanContext().TraceID(), tc.Equals, traceID)
	c.Check(trace.SpanFromContext(ctx).SpanContext().SpanID(), tc.Equals, spanID)
}

func (s *tracerSuite) TestTracerStartContextShouldBeCanceled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, cancel := context.WithCancel(c.Context())

	// cancel the context straight away
	cancel()

	ctx, span := tracer.Start(ctx, "foo")
	defer span.End()

	select {
	case <-ctx.Done():
	default:
		c.Fatalf("context should be done")
	}
}

func (s *tracerSuite) TestTracerAddEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	s.span.EXPECT().AddEvent(gomock.Any(), gomock.Any()).AnyTimes()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.AddEvent("bar")
	span.AddEvent("baz", coretrace.StringAttr("qux", "quux"))
}

func (s *tracerSuite) TestTracerRecordErrorWithNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.RecordError(nil)
}

func (s *tracerSuite) TestTracerRecordError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	s.span.EXPECT().RecordError(gomock.Any(), gomock.Any()).AnyTimes()
	s.span.EXPECT().SetStatus(gomock.Any(), gomock.Any()).AnyTimes()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.RecordError(errors.Errorf("boom"))
}

func (s *tracerSuite) TestBuildRequestContextWithBackgroundContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := w.(*tracer).buildRequestContext(c.Context())
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContextWithBrokenTraceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "foo", "bar", 0)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContextWithBrokenSpanID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "80f198ee56343ba864fe8b2a57d3eff7", "bar", 0)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "80f198ee56343ba864fe8b2a57d3eff7", "ff00000000000000", 1)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)

	span := trace.SpanContextFromContext(ctx)
	c.Check(span.IsRemote(), tc.IsTrue)
}

func (s *tracerSuite) newTracer(c *tc.C) TrackedTracer {
	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	newClient := func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, float64, time.Duration, logger.Logger) (Client, ClientTracerProvider, ClientTracer, error) {
		return s.client, s.clientTracerProvider, s.clientTracer, nil
	}
	tracer, err := NewTracerWorker(c.Context(), ns, "http://meshuggah.com", "", "", false, false, 0.42, time.Second, s.logger, newClient)
	c.Assert(err, tc.ErrorIsNil)
	return tracer
}
