// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/worker"
)

// NewMetricsManager creates a runner that will run the metricsmanagement workers.
func NewMetricsManager(client metricsmanager.MetricsManagerClient) (worker.Runner, error) {
	// Periodic workers automatically retry so none should return an error. If they do
	// it's ok to restart them individually.
	isFatal := func(error) bool {
		return false
	}
	// All errors are equal
	moreImportant := func(error, error) bool {
		return false
	}
	runner := worker.NewRunner(isFatal, moreImportant)
	err := runner.StartWorker("sender", func() (worker.Worker, error) {
		return NewSender(client), nil
	})

	if err != nil {
		return nil, errors.Trace(err)
	}
	err = runner.StartWorker("cleanup", func() (worker.Worker, error) {
		return NewCleanup(client), nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return runner, nil
}
