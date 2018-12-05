// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"github.com/armon/go-metrics"
	pmetrics "github.com/armon/go-metrics/prometheus"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterMetrics connects the metrics gathered in the
// hashicorp/raft library code with our prometheus collector.
func RegisterMetrics(registerer prometheus.Registerer) error {
	sink, err := pmetrics.NewPrometheusSink()
	if err != nil {
		return errors.Trace(err)
	}
	registerer.Unregister(sink)
	if err := registerer.Register(sink); err != nil {
		return errors.Trace(err)
	}
	_, err = metrics.NewGlobal(metrics.DefaultConfig("jujumetrics"), sink)
	return errors.Trace(err)
}
