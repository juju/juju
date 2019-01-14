// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"github.com/armon/go-metrics"
	pmetrics "github.com/armon/go-metrics/prometheus"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// registerMetrics connects the metrics gathered in the
// hashicorp/raft library code with our prometheus collector.
func registerMetrics(registerer prometheus.Registerer) (prometheus.Collector, error) {
	sink, err := pmetrics.NewPrometheusSink()
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, err = metrics.NewGlobal(metrics.DefaultConfig("juju"), sink)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := registerer.Register(sink); err != nil {
		return nil, errors.Trace(err)
	}
	return sink, nil
}
