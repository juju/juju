// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/metricsmanager"
	jworker "github.com/juju/juju/worker"
)

// NewMetricsManager creates a runner that will run the metricsmanagement workers.
func newMetricsManager(client metricsmanager.MetricsManagerClient, notify chan string, logger Logger) (*worker.Runner, error) {
	// TODO(fwereade): break this out into separate manifolds (with their own facades).

	// Periodic workers automatically retry so none should return an error. If they do
	// it's ok to restart them individually.
	isFatal := func(error) bool {
		return false
	}
	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:      isFatal,
		RestartDelay: jworker.RestartDelay,
	})
	err := runner.StartWorker("sender", func() (worker.Worker, error) {
		return newSender(client, notify, logger), nil
	})

	if err != nil {
		return nil, errors.Trace(err)
	}

	err = runner.StartWorker("cleanup", func() (worker.Worker, error) {
		return newCleanup(client, notify, logger), nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return runner, nil
}
