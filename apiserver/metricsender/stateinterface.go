// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains functions for sending
// metrics from a controller to a remote metric collector.
package metricsender

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// MetricsSenderBackend defines methods provided by a state
// instance used by the metrics sender apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type MetricsSenderBackend interface {
	MetricsManager() (*state.MetricsManager, error)
	MetricsToSend(batchSize int) ([]*state.MetricBatch, error)
	SetMetricBatchesSent(batchUUIDs []string) error
	CountOfUnsentMetrics() (int, error)
	CountOfSentMetrics() (int, error)
}

// ModelBackend contains additional methods that are used by the metrics sender.
type ModelBackend interface {
	MetricsSenderBackend

	Unit(name string) (*state.Unit, error)
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
}
