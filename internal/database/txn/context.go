// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import "context"

// contextKey is a type used for context keys in this package.
type contextKey string

const (
	metricsKey contextKey = "metrics"
)

// MetricErrorType is the type of error that should be recorded.
type MetricErrorType string

const (
	sqliteRetryableError MetricErrorType = "sqlite-retryable-error"
)

// Metrics is the interface that must be implemented by the metrics collector.
type Metrics interface {
	// RecordError records an error of the given error type.
	RecordError(MetricErrorType)
}

// WithMetrics returns a new context with the given metrics.
func WithMetrics(ctx context.Context, metrics Metrics) context.Context {
	return context.WithValue(ctx, metricsKey, metrics)
}

// MetricsFromContext returns the metrics from the given context.
// If no metrics are found, then a noops metrics is returned.
func MetricsFromContext(ctx context.Context) Metrics {
	metrics, _ := ctx.Value(metricsKey).(Metrics)
	if metrics == nil {
		return noopsMetrics{}
	}
	return metrics
}

type noopsMetrics struct{}

func (noopsMetrics) RecordError(MetricErrorType) {}
