// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains functions for sending
// metrics from a controller to a remote metric collector.
package metricsender

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// ModelBackend contains methods that are used by the metrics sender.
type ModelBackend interface {
	MetricsManager() (*state.MetricsManager, error)
	MetricsToSend(batchSize int) ([]*state.MetricBatch, error)
	SetMetricBatchesSent(batchUUIDs []string) error
	CountOfUnsentMetrics() (int, error)
	CountOfSentMetrics() (int, error)
	CleanupOldMetrics() error

	Name() string
	Unit(name string) (*state.Unit, error)
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	ControllerConfig() (controller.Config, error)
	SetModelMeterStatus(string, string) error
}
