// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type clientSuite struct {
	baseSuite

	bsp *MockSpanProcessor
}

func TestClientSuite(t *testing.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	s.bsp = NewMockSpanProcessor(ctrl)
	return ctrl
}

// --- tailSamplingProcessor.OnEnd ---

func (s *clientSuite) TestOnEndExportsErrorSpans(c *tc.C) {
	defer s.setupMocks(c).Finish()

	proc := &tailSamplingProcessor{
		bsp:       s.bsp,
		logger:    s.logger,
		threshold: time.Second,
	}

	// A span with an error status must always be exported, regardless of
	// duration.
	span := tracetest.SpanStub{
		Status:    sdktrace.Status{Code: codes.Error, Description: "boom"},
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Millisecond),
	}.Snapshot()

	s.bsp.EXPECT().OnEnd(gomock.Any())

	proc.OnEnd(span)
}

func (s *clientSuite) TestOnEndExportsLongSpans(c *tc.C) {
	defer s.setupMocks(c).Finish()

	proc := &tailSamplingProcessor{
		bsp:       s.bsp,
		logger:    s.logger,
		threshold: 100 * time.Millisecond,
	}

	start := time.Now()
	end := start.Add(200 * time.Millisecond)

	span := tracetest.SpanStub{
		Status:    sdktrace.Status{Code: codes.Ok},
		StartTime: start,
		EndTime:   end,
	}.Snapshot()

	s.bsp.EXPECT().OnEnd(gomock.Any())

	proc.OnEnd(span)
}

func (s *clientSuite) TestOnEndDropsShortSpans(c *tc.C) {
	defer s.setupMocks(c).Finish()

	proc := &tailSamplingProcessor{
		bsp:       s.bsp,
		logger:    s.logger,
		threshold: time.Second,
	}

	start := time.Now()
	end := start.Add(10 * time.Millisecond)

	span := tracetest.SpanStub{
		Status:    sdktrace.Status{Code: codes.Ok},
		StartTime: start,
		EndTime:   end,
	}.Snapshot()

	// bsp.OnEnd must NOT be called for short, non-error spans.
	proc.OnEnd(span)
}

// --- tailSamplingProcessor.Shutdown / ForceFlush ---

func (s *clientSuite) TestShutdownDelegates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	proc := &tailSamplingProcessor{
		bsp:       s.bsp,
		logger:    s.logger,
		threshold: time.Second,
	}

	ctx := context.Background()
	s.bsp.EXPECT().Shutdown(ctx).Return(nil)

	c.Check(proc.Shutdown(ctx), tc.ErrorIsNil)
}

func (s *clientSuite) TestForceFlushDelegates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	proc := &tailSamplingProcessor{
		bsp:       s.bsp,
		logger:    s.logger,
		threshold: time.Second,
	}

	ctx := context.Background()
	s.bsp.EXPECT().ForceFlush(ctx).Return(nil)

	c.Check(proc.ForceFlush(ctx), tc.ErrorIsNil)
}

// --- tailSamplingProcessor integration test ---

func (s *clientSuite) TestTailSamplingProcessorIntegration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Use an in-memory exporter to capture exported spans. Wrap it in
	// the tailSamplingProcessor so we can verify the drop/export logic
	// with real spans.
	exporter := tracetest.NewInMemoryExporter()
	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithMaxExportBatchSize(1),
		sdktrace.WithMaxQueueSize(10),
	)

	threshold := 100 * time.Millisecond
	proc := &tailSamplingProcessor{
		bsp:       bsp,
		logger:    s.logger,
		threshold: threshold,
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(proc),
	)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tr := tp.Tracer("test")

	// 1. A short, non-error span should be dropped.
	_, span1 := tr.Start(context.Background(), "short-span")
	span1.End()
	c.Assert(tp.ForceFlush(context.Background()), tc.ErrorIsNil)
	c.Check(len(exporter.GetSpans()), tc.Equals, 0,
		tc.Commentf("short non-error span should have been dropped"))

	// 2. An error span should be exported regardless of duration.
	_, span2 := tr.Start(context.Background(), "error-span")
	span2.SetStatus(codes.Error, "boom")
	span2.End()
	c.Assert(tp.ForceFlush(context.Background()), tc.ErrorIsNil)
	spans := exporter.GetSpans()
	c.Assert(len(spans), tc.Equals, 1)
	c.Check(spans[0].Name, tc.Equals, "error-span")
	exporter.Reset()

	// 3. A long, non-error span should be exported.
	_, span3 := tr.Start(context.Background(), "long-span")
	time.Sleep(threshold + 50*time.Millisecond)
	span3.End()
	c.Assert(tp.ForceFlush(context.Background()), tc.ErrorIsNil)
	spans = exporter.GetSpans()
	c.Assert(len(spans), tc.Equals, 1)
	c.Check(spans[0].Name, tc.Equals, "long-span")
}

// --- newResource ---

func (s *clientSuite) TestNewResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	res := newResource("juju-controller", "agent/controller/machine-0")
	c.Assert(res, tc.NotNil)

	attrs := make(map[string]string, res.Len())
	for iter := res.Iter(); iter.Next(); {
		kv := iter.Attribute()
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	c.Check(attrs["service.name"], tc.Equals, "juju-controller")
	c.Check(attrs["service.instance.id"], tc.Equals, "agent/controller/machine-0")
	c.Check(attrs["service.version"], tc.Not(tc.Equals), "")
}
