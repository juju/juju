// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/metrics/spool"
)

var (
	logger               = loggo.GetLogger("juju.worker.metrics.sender")
	newMetricAdderClient = func(apiCaller base.APICaller) metricsadder.MetricsAdderClient {
		return metricsadder.NewClient(apiCaller)
	}
)

const (
	period = time.Minute * 5
)

// ManifoldConfig defines configuration of a metric sender manifold.
type ManifoldConfig struct {
	APICallerName    string
	MetricsSpoolName string
}

// Manifold creates a metric sender manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.MetricsSpoolName,
		},
		Start: start,
	}
}

func start(getResource dependency.GetResourceFunc) (worker.Worker, error) {
	var apicaller base.APICaller
	var factory spool.MetricFactory
	err := getResource(unit.APICallerName, &apicaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = getResource(unit.MetricSpoolName, &factory)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := newMetricAdderClient(apicaller)

	s := newSender(client, factory)
	return worker.NewPeriodicWorker(s.Do, period, worker.NewTimer), nil
}
