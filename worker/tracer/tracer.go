// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"
	"time"

	"github.com/juju/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/tomb.v2"

	coretracer "github.com/juju/juju/core/tracer"
)

type tracer struct {
	tomb tomb.Tomb

	namespace      string
	client         otlptrace.Client
	clientProvider *sdktrace.TracerProvider
	clientTracer   trace.Tracer
	logger         Logger
}

// NewTracerWorker returns a new tracer worker.
func NewTracerWorker(ctx context.Context, namespace string, logger Logger) (TrackedTracer, error) {
	client, clientProvider, clientTracer, err := newClient(ctx, namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &tracer{
		namespace:      namespace,
		client:         client,
		clientProvider: clientProvider,
		clientTracer:   clientTracer,
		logger:         logger,
	}

	t.tomb.Go(t.loop)
	return t, nil
}

// Kill implements the worker.Worker interface.
func (t *tracer) Kill() {
	t.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (t *tracer) Wait() error {
	return t.tomb.Wait()
}

// Start creates a span and a context.Context containing the newly-created span.
func (t *tracer) Start(ctx context.Context, name string, opts ...coretracer.Option) (context.Context, coretracer.Span) {
	o := coretracer.NewTracerOptions()
	for _, opt := range opts {
		opt(o)
	}

	// Allows the override of the name.
	if n := o.Name(); n != "" {
		name = n
	}

	var attrs []attribute.KeyValue
	for k, v := range o.Attributes() {
		attrs = append(attrs, attribute.String(k, v))
	}

	var (
		cancel context.CancelFunc
		span   trace.Span
	)
	ctx, cancel = t.scopedContext(ctx)
	ctx, span = t.clientTracer.Start(ctx, name, trace.WithAttributes(attrs...))

	return ctx, &managedSpan{
		span:       span,
		cancel:     cancel,
		stackTrace: o.StackTrace(),
	}
}

func (t *tracer) loop() error {
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		if err := t.client.Stop(ctx); err != nil {
			t.logger.Infof("failed to stop client: %v", err)
		}

		if err := t.clientProvider.Shutdown(ctx); err != nil {
			t.logger.Infof("failed to shutdown provider: %v", err)
		}
	}()

	<-t.tomb.Dying()
	return tomb.ErrDying
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *tracer) scopedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	return w.tomb.Context(ctx), cancel
}

func newClient(ctx context.Context, namespace string) (otlptrace.Client, *sdktrace.TracerProvider, trace.Tracer, error) {
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("192.168.0.60:4317"),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(namespace)),
	)

	return client, tp, tp.Tracer(namespace), nil
}

func newResource(namespace string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(namespace),
		semconv.ServiceVersion("0.0.1"),
	)
}

type managedSpan struct {
	span       trace.Span
	cancel     context.CancelFunc
	stackTrace bool
}

func (s *managedSpan) End() {
	defer s.cancel()
	s.span.End(trace.WithStackTrace(s.stackTrace))
}
